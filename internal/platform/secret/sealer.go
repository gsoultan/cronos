package secret

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
)

/*
Sealer encrypts a secret for a database to hold, and opens it again.

A key somebody types into the portal has to be kept somewhere, and the only
somewhere every replica of a deployment shares is the store. Kept there in the
clear it is in every backup of that database, every read replica, and every
dump somebody takes to reproduce a bug — the exposure a ${secret:…} reference
exists to keep out of definition files, moved one layer down.

So it is sealed with a key the database never sees: AES-256-GCM under a key
derived from CRONOS_SECRETS_KEY. A copy of the store alone is ciphertext, and a
copy of the key alone opens nothing.

Every value is bound to where it belongs. The organisation, project and name are
authenticated with it, so a value copied into another project's row — by
somebody who can write to the database and cannot read the key — does not open
there. Without that, write access to one table would be a way to move a
customer's warehouse password into a project the attacker edits.
*/
type Sealer struct {
	current sealKey
	/*
	   retired open what they sealed and seal nothing.

	   The rotation the signing key has, and for the same reason: without it a
	   new key makes every stored secret unreadable at once, which means the key
	   never rotates. With it, the new key goes in, the old one moves here, the
	   next boot seals everything again under the new one, and the old one can
	   go.
	*/
	retired []sealKey
}

type sealKey struct {
	// id says which key sealed a value, so opening one is a lookup rather than
	// a trial of every key, and a value sealed by a key nobody configured is
	// reported as that rather than as tampering.
	id   [keyIDBytes]byte
	aead cipher.AEAD
}

const (
	// sealVersion leads every sealed value. A later format can then be told
	// apart from this one, rather than failing to authenticate as though it
	// were a damaged value of this one.
	sealVersion byte = 1
	keyIDBytes       = 8
	headerBytes      = 1 + keyIDBytes
	// MinSealKeyBytes is the shortest key accepted, the same floor the signing
	// key has. HKDF will stretch anything, including a word somebody chose.
	MinSealKeyBytes = 32
)

// NewSealer returns a Sealer over key, also opening what the retired keys
// sealed. A key that is too short is refused: it is raised at startup, where
// somebody can see it.
func NewSealer(key []byte, retired ...[]byte) (*Sealer, error) {
	current, err := derive(key)
	if err != nil {
		return nil, err
	}
	s := &Sealer{current: current}
	for _, r := range retired {
		k, err := derive(r)
		if err != nil {
			return nil, fmt.Errorf("a previous secrets key: %w", err)
		}
		s.retired = append(s.retired, k)
	}
	return s, nil
}

// derive turns a configured key into the cipher and the id. Two derivations
// from one key, so the id says which key sealed a value and nothing about the
// key itself.
func derive(key []byte) (sealKey, error) {
	if len(key) < MinSealKeyBytes {
		return sealKey{}, fmt.Errorf("%w: %d bytes, need at least %d",
			ErrWeakKey, len(key), MinSealKeyBytes)
	}
	enc, err := hkdf.Key(sha256.New, key, nil, "cronos secret sealing v1", 32)
	if err != nil {
		return sealKey{}, err
	}
	id, err := hkdf.Key(sha256.New, key, nil, "cronos secret key id v1", keyIDBytes)
	if err != nil {
		return sealKey{}, err
	}
	block, err := aes.NewCipher(enc)
	if err != nil {
		return sealKey{}, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return sealKey{}, err
	}
	k := sealKey{aead: aead}
	copy(k.id[:], id)
	return k, nil
}

/*
Seal encrypts value, bound to the parts that say where it belongs.

A random nonce per value. GCM's limit with random nonces is around four billion
values under one key, and a deployment's secrets are counted in dozens.
*/
func (s *Sealer) Seal(value string, bound ...string) []byte {
	k := s.current
	out := make([]byte, 0, headerBytes+k.aead.NonceSize()+len(value)+k.aead.Overhead())
	out = append(out, sealVersion)
	out = append(out, k.id[:]...)
	nonce := make([]byte, k.aead.NonceSize())
	// crypto/rand does not fail: it aborts the process where it cannot read,
	// which is the right answer for a nonce.
	_, _ = rand.Read(nonce)
	out = append(out, nonce...)
	return k.aead.Seal(out, nonce, []byte(value), binding(out[:headerBytes], bound))
}

// Open decrypts what Seal made, given the same parts. Anything else — another
// project's value, a damaged one, one sealed by a key this process was not
// given — is ErrSealed, and nothing about which.
func (s *Sealer) Open(sealed []byte, bound ...string) (string, error) {
	k, ok := s.keyOf(sealed)
	if !ok {
		return "", ErrSealed
	}
	body := sealed[headerBytes:]
	n := k.aead.NonceSize()
	if len(body) < n+k.aead.Overhead() {
		return "", ErrSealed
	}
	plain, err := k.aead.Open(nil, body[:n], body[n:], binding(sealed[:headerBytes], bound))
	if err != nil {
		return "", ErrSealed
	}
	return string(plain), nil
}

/*
Stale reports whether a value was sealed by a retired key.

The rotation's second half: a value the current key did not seal is sealed again
under it at startup, and once nothing is stale the retired key can be removed
from the configuration without losing anything.
*/
func (s *Sealer) Stale(sealed []byte) bool {
	return len(sealed) >= headerBytes && !bytes.Equal(sealed[1:headerBytes], s.current.id[:])
}

// keyOf finds the key a value names in its header.
func (s *Sealer) keyOf(sealed []byte) (sealKey, bool) {
	if len(sealed) < headerBytes || sealed[0] != sealVersion {
		return sealKey{}, false
	}
	id := sealed[1:headerBytes]
	if bytes.Equal(id, s.current.id[:]) {
		return s.current, true
	}
	for _, k := range s.retired {
		if bytes.Equal(id, k.id[:]) {
			return k, true
		}
	}
	return sealKey{}, false
}

/*
binding is what a value is authenticated against besides itself: the header, so
the version and key id cannot be rewritten, and each part length-prefixed.

Prefixed rather than joined with a separator, because a separator is a character
an organisation or a secret name might contain, and "a" + "b/c" must not bind the
same as "a/b" + "c".
*/
func binding(header []byte, parts []string) []byte {
	out := append([]byte(nil), header...)
	for _, p := range parts {
		out = binary.AppendUvarint(out, uint64(len(p)))
		out = append(out, p...)
	}
	return out
}
