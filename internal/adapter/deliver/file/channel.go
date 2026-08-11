package file

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/gsoultan/cronos/internal/app/burst"
)

// unsafe matches anything that must not reach a path.
//
// A filename is resolved from a row of somebody's database — a customer name,
// an invoice id — so it is attacker-influenced in exactly the way a path
// should never be. Separators and dots are replaced rather than rejected,
// because refusing to deliver a statement because a customer is called
// "A/S Nordisk" would be the wrong answer.
var unsafe = regexp.MustCompile(`[^\w.\-]+`)

// Channel writes each delivery under a directory.
type Channel struct {
	root string
}

// New returns a Channel writing under root.
func New(root string) *Channel { return &Channel{root: root} }

// Name is what a schedule's `via` matches.
func (c *Channel) Name() string { return "file" }

// Deliver writes the document to root/<to>/<filename>.
func (c *Channel) Deliver(_ context.Context, d burst.Delivery) error {
	name := clean(d.Filename)
	if name == "" {
		return fmt.Errorf("file: delivery has no filename")
	}

	dir := filepath.Join(c.root, clean(d.To))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, name), d.Document, 0o600)
}

// clean reduces a resolved template to one safe path segment.
//
// The result never contains a separator and never begins with a dot, so it
// cannot escape the root, cannot become a hidden file, and cannot be "..".
func clean(s string) string {
	s = unsafe.ReplaceAllString(strings.TrimSpace(s), "-")
	s = strings.Trim(s, ".-")
	if strings.Contains(s, "..") {
		s = strings.ReplaceAll(s, "..", "-")
	}
	return s
}
