package boot

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/gsoultan/cronos/internal/platform/config"
	"github.com/gsoultan/cronos/internal/platform/token"
)

const (
	signingKey = "the-key-that-never-leaves-cronos-xx"
	hostKey    = "the-key-a-host-application-is-given"
	oldHostKey = "the-key-hosts-were-given-last-year-"
)

func embedSetup(t *testing.T, cfg config.Server) (*token.Signer, *token.Signer, string, error) {
	t.Helper()
	cfg.SigningKey = []byte(signingKey)
	signer, err := token.NewSigner(cfg.SigningKey)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	embed, err := embedSignerFor(cfg, signer, slog.New(slog.NewTextHandler(&out, nil)))
	return signer, embed, out.String(), err
}

func embedClaims() token.Claims {
	return token.Claims{Audience: token.Embed, Org: "acme", Project: "finance"}
}

func TestAnEmbedKeyOfItsOwnIsWhatEmbedTokensAreSignedWith(t *testing.T) {
	signer, embed, said, err := embedSetup(t, config.Server{EmbedKey: []byte(hostKey)})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(said, "CRONOS_SIGNING_KEY") {
		t.Errorf("warned about the signing key with an embed key set: %s", said)
	}

	tok, err := embed.Mint(embedClaims(), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := embed.Verify(tok, token.Embed); err != nil {
		t.Fatalf("the embed signer does not verify its own token: %v", err)
	}
	if _, err := signer.Verify(tok, token.Embed); err == nil {
		t.Fatal("the signing key verified a token signed with the embed key: they are the same key")
	}
}

// Said every time it starts, because it is the configuration in which a host
// application still holds the key to the portal.
func TestWithoutAnEmbedKeyStartupSaysWhatThatMeans(t *testing.T) {
	_, embed, said, err := embedSetup(t, config.Server{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(said, "CRONOS_EMBED_KEY") || !strings.Contains(said, "level=WARN") {
		t.Errorf("no warning naming CRONOS_EMBED_KEY: %q", said)
	}
	// Confined all the same: what comes back never mints a portal session,
	// whichever key it holds.
	portal := embedClaims()
	portal.Audience = token.Portal
	if _, err := embed.Mint(portal, time.Hour); err == nil {
		t.Fatal("the embed signer minted a portal token")
	}
}

func TestAnEmbedKeyThatIsASigningKeyIsRefused(t *testing.T) {
	for name, cfg := range map[string]config.Server{
		"the signing key itself": {EmbedKey: []byte(signingKey)},
		// The portal still accepts a retired key for as long as a rotation
		// runs, so a host given it could sign a session for that long.
		"a retired signing key": {EmbedKey: []byte(oldHostKey), PreviousKeys: [][]byte{[]byte(oldHostKey)}},
	} {
		if _, _, _, err := embedSetup(t, cfg); err == nil {
			t.Errorf("%s was accepted as the embed key", name)
		}
	}
}

// Moving hosts off the signing key, and any rotation after: tokens signed with
// the previous key keep working while hosts change over, for embed only.
func TestAPreviousEmbedKeyStillOpensReportsDuringARotation(t *testing.T) {
	_, embed, _, err := embedSetup(t, config.Server{
		EmbedKey: []byte(hostKey), PreviousEmbedKeys: [][]byte{[]byte(oldHostKey)},
	})
	if err != nil {
		t.Fatal(err)
	}
	old, _ := token.NewSigner([]byte(oldHostKey))
	tok, _ := old.Mint(embedClaims(), time.Hour)
	if _, err := embed.Verify(tok, token.Embed); err != nil {
		t.Fatalf("a token from the previous embed key was refused mid-rotation: %v", err)
	}

	if _, _, _, err := embedSetup(t, config.Server{PreviousEmbedKeys: [][]byte{[]byte(oldHostKey)}}); err == nil {
		t.Error("previous embed keys with no embed key started anyway")
	}
	if _, _, _, err := embedSetup(t, config.Server{EmbedKey: []byte("short")}); err == nil {
		t.Error("an embed key too short to sign with was accepted")
	}
}
