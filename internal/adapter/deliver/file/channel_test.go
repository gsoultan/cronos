package file_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gsoultan/cronos/internal/adapter/deliver/file"
	"github.com/gsoultan/cronos/internal/app/burst"
)

/*
Writing a delivered document to a directory.

The package had no tests, and the thing it does that needs them is not the
writing. Both path segments — the recipient and the filename — are resolved
from a row of somebody's database: a customer name, an invoice id. That makes
them attacker-influenced in exactly the way a path never should be, and this is
the channel every deployment can use without configuring anything, so it is the
one most likely to be pointed at a directory that matters.

Neutralised rather than refused, and the distinction is the design: declining
to deliver a statement because a customer is called "A/S Nordisk" would be the
wrong answer, so separators become dashes and the delivery goes out.
*/

func deliver(t *testing.T, root string, d burst.Delivery) error {
	t.Helper()
	return file.New(root).Deliver(context.Background(), d)
}

// written returns every file under root, as paths relative to it.
func written(t *testing.T, root string) []string {
	t.Helper()

	var out []string
	err := filepath.WalkDir(root, func(path string, e os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !e.IsDir() {
			rel, _ := filepath.Rel(root, path)
			out = append(out, rel)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestADocumentLandsUnderTheRecipient(t *testing.T) {
	root := t.TempDir()

	err := deliver(t, root, burst.Delivery{
		To: "c-1", Filename: "statement.pdf", Document: []byte("%PDF pretend"),
	})
	if err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(filepath.Join(root, "c-1", "statement.pdf"))
	if err != nil {
		t.Fatalf("nothing was written where it should be: %v", err)
	}
	if string(got) != "%PDF pretend" {
		t.Errorf("the document is %q", got)
	}
}

/*
Nothing a row contains can escape the root.

Both segments are checked, because a burst resolves both from the same row and
whichever one an author forgot to think about is the one that matters.
*/
func TestNothingResolvedFromARowCanEscapeTheRoot(t *testing.T) {
	for _, c := range []struct{ name, to, filename string }{
		{"traversal in the filename", "c-1", "../../escaped.pdf"},
		{"traversal in the recipient", "../../..", "statement.pdf"},
		{"traversal in both", "../x", "../y.pdf"},
		{"an absolute filename", "c-1", "/etc/cron.d/payload"},
		{"an absolute recipient", "/etc", "statement.pdf"},
		{"separators inside a name", "A/S Nordisk", "statement.pdf"},
		{"a windows separator", `c-1`, `..\..\escaped.pdf`},
	} {
		t.Run(c.name, func(t *testing.T) {
			root := t.TempDir()
			// A sibling the traversal would reach if it worked.
			outside := filepath.Join(root, "..", "escaped.pdf")
			t.Cleanup(func() { os.Remove(outside) })

			if err := deliver(t, root, burst.Delivery{
				To: c.to, Filename: c.filename, Document: []byte("x"),
			}); err != nil {
				// Refusing is a fine answer too; escaping is not.
				return
			}

			for _, p := range written(t, root) {
				if strings.Contains(p, "..") {
					t.Errorf("wrote %q, which still contains a traversal", p)
				}
				if filepath.IsAbs(p) {
					t.Errorf("wrote an absolute path %q", p)
				}
			}
			if _, err := os.Stat(outside); err == nil {
				t.Fatalf("%s: a document was written outside the root", c.name)
			}
		})
	}
}

// A customer whose name is punctuation still gets their statement, under a
// name that is not a hidden file.
func TestANameThatIsAllPunctuationStillDeliversVisibly(t *testing.T) {
	root := t.TempDir()

	if err := deliver(t, root, burst.Delivery{
		To: "...", Filename: ".hidden.pdf", Document: []byte("x"),
	}); err != nil {
		return // refusing is acceptable
	}

	for _, p := range written(t, root) {
		for _, seg := range strings.Split(p, string(filepath.Separator)) {
			if strings.HasPrefix(seg, ".") {
				t.Errorf("wrote %q, which is a hidden file", p)
			}
		}
	}
}

func TestADeliveryWithNoFilenameIsRefused(t *testing.T) {
	root := t.TempDir()

	for _, name := range []string{"", "   ", "...", "---"} {
		if err := deliver(t, root, burst.Delivery{
			To: "c-1", Filename: name, Document: []byte("x"),
		}); err == nil {
			t.Errorf("filename %q was accepted, and reduces to nothing", name)
		}
	}
}

// A statement is somebody's billing data sitting on a shared filesystem.
func TestADeliveredDocumentIsNotWorldReadable(t *testing.T) {
	root := t.TempDir()

	if err := deliver(t, root, burst.Delivery{
		To: "c-1", Filename: "statement.pdf", Document: []byte("x"),
	}); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(filepath.Join(root, "c-1", "statement.pdf"))
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode&0o077 != 0 {
		t.Errorf("the document is mode %04o, want nothing for group or other", mode)
	}
}

// The channel's name is what a schedule's `via` matches, so it is part of the
// definition format rather than an implementation detail.
func TestTheChannelIsNamedFile(t *testing.T) {
	if got := file.New(t.TempDir()).Name(); got != "file" {
		t.Errorf("Name is %q, want file", got)
	}
}
