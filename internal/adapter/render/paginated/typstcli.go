package paginated

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// TypstCLI compiles by running the typst binary.
//
// A subprocess rather than a linked library: typst is Rust, so linking it
// means cgo in every build of every binary that might render. A fork costs
// single-digit milliseconds against a render measured in hundreds, and it
// buys a crash boundary — a typesetter that runs out of memory on one
// pathological statement takes down that render and not the server.
type TypstCLI struct {
	// Bin is the executable. Empty means "typst" on PATH.
	Bin string
	// Timeout bounds one compile. Zero means DefaultTimeout. Unbounded is not
	// an option: a document is user-authored input to a typesetter.
	Timeout time.Duration
	// FontDir is the only directory fonts are loaded from. Empty means the
	// faces bundled with typst and nothing else.
	//
	// System fonts are always ignored. Whether a PDF archived today can be
	// reproduced next year must not depend on what was installed on the host
	// that rendered it, and "the statement looks different on the new
	// container" is not a bug anyone enjoys finding by eye.
	FontDir string
}

// DefaultTimeout is generous for a statement and still short enough that a
// wedged render fails a delivery rather than a shift.
const DefaultTimeout = 30 * time.Second

// ErrTypstMissing is returned when the binary cannot be found, separately from
// a failed compile — one is an install problem and the other is a document
// problem, and an operator should not have to read a stack trace to tell them
// apart.
var ErrTypstMissing = errors.New("paginated: typst binary not found")

func (t TypstCLI) bin() string {
	if t.Bin != "" {
		return t.Bin
	}
	if env := os.Getenv("CRONOS_TYPST_BIN"); env != "" {
		return env
	}
	return "typst"
}

// Compile typesets root/main into out.
//
// --root confines every path the document can reach to the directory the
// renderer just built. Nothing else on the filesystem is addressable, which is
// what makes a user-authored header template safe to typeset.
func (t TypstCLI) Compile(ctx context.Context, root, main, out string) error {
	timeout := t.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	args := []string{"compile", "--root", root, "--ignore-system-fonts"}
	if t.FontDir != "" {
		args = append(args, "--font-path", t.FontDir)
	}
	cmd := exec.CommandContext(ctx, t.bin(), append(args, main, out)...)
	cmd.Dir = root
	stderr := &strings.Builder{}
	cmd.Stderr = stderr

	if err := cmd.Run(); err != nil {
		return t.wrap(ctx, err, stderr.String())
	}
	return nil
}

func (t TypstCLI) wrap(ctx context.Context, err error, stderr string) error {
	if errors.Is(err, exec.ErrNotFound) {
		return fmt.Errorf("%w: %q — install it or set CRONOS_TYPST_BIN", ErrTypstMissing, t.bin())
	}
	if ctx.Err() != nil {
		return fmt.Errorf("paginated: typst timed out after %s", t.Timeout)
	}
	return fmt.Errorf("paginated: typst failed: %w: %s", err, strings.TrimSpace(stderr))
}
