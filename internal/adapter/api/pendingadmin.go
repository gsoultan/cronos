package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/gsoultan/cronos/internal/core/identity"
)

/*
The administrator first-run setup collected, handed to the boot that can create
them.

Two processes are involved and they cannot both talk to the database. The
process running setup has no store — the DSN it would use arrived in the same
request — so opening one there would mean running migrations from an endpoint
that has just authenticated a stranger holding a file. The configured server
that starts next has a store, a signer, and every check that normally guards
account creation, so it is where the account is made.

What passes between them is this file, and the one rule about it is that the
password never does. It is hashed by setup with the same function the store
would have used, so what sits on disk between the two boots is a bcrypt hash
rather than something somebody typed.

Deleted the moment it is consumed. A file holding a credential for an account
that already exists is a credential nobody is watching.
*/

// PendingName is the file, beside the configuration it was written with.
const PendingName = "pending-admin.json"

// PendingAdmin is the first administrator, waiting for a database.
type PendingAdmin struct {
	Email string `json:"email"`
	Name  string `json:"name,omitempty"`
	Org   string `json:"org"`
	// Hash is bcrypt, never a password. See the package comment.
	Hash string `json:"hash"`
	// Project is where they administer, which is also the project the
	// configuration names.
	Project string `json:"project"`
}

// pendingPath is the file beside a configuration.
func pendingPath(configPath string) string {
	return filepath.Join(filepath.Dir(configPath), PendingName)
}

/*
writePendingAdmin records the administrator for the next boot to create.

The password is hashed here rather than passed through, so it exists in memory
for the length of one request and never on disk.
*/
func writePendingAdmin(configPath string, in firstRunRequest) error {
	hash, err := identity.Hash(in.Password)
	if err != nil {
		return err
	}

	raw, err := json.Marshal(PendingAdmin{
		Email: strings.ToLower(strings.TrimSpace(in.Email)),
		Name:  strings.TrimSpace(in.Name),
		Org:   in.Org, Project: in.Project, Hash: hash,
	})
	if err != nil {
		return err
	}

	path := pendingPath(configPath)
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	// 0600: a bcrypt hash is not a password and is still worth nobody else's
	// time offline.
	return os.WriteFile(path, raw, 0o600)
}

/*
ReadPendingAdmin reads an administrator left by setup, if there is one.

A missing file is the ordinary case — every boot after the first — so it is not
an error. A file that cannot be read is, because the alternative is a
deployment that silently starts with no way in and an operator who was told an
account had been made.
*/
func ReadPendingAdmin(configPath string) (PendingAdmin, bool, error) {
	path := pendingPath(configPath)

	raw, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return PendingAdmin{}, false, nil
	}
	if err != nil {
		return PendingAdmin{}, false, fmt.Errorf("setup: reading %s: %w", path, err)
	}

	var p PendingAdmin
	if err := json.Unmarshal(raw, &p); err != nil {
		return PendingAdmin{}, false, fmt.Errorf("setup: %s is unreadable: %w", path, err)
	}
	if p.Email == "" || p.Hash == "" {
		return PendingAdmin{}, false, fmt.Errorf("setup: %s names no administrator", path)
	}
	return p, true, nil
}

// ClearPendingAdmin removes the file once the account exists.
func ClearPendingAdmin(configPath string) error {
	err := os.Remove(pendingPath(configPath))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}
