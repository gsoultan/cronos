package build

import (
	"strings"
	"testing"
)

/*
The one thing this must never do is answer with nothing.

An operator asks which build a pod is only when something is wrong with it, and
an empty answer at that moment is worse than "unknown": a blank field reads as a
bug in the thing being asked rather than as a build with no revision stamped
into it, and the next half hour goes to the wrong question.

`go test` runs a binary the toolchain builds without a VCS stamp, so this is
exercised on exactly the path that produces "unknown" — which is the branch
worth having a test for. That the shipped container knows its own commit is
checked by the image job in CI, because the answer there comes from a build arg
rather than from the repository — .dockerignore excludes .git on purpose.
*/
func TestTheBuildAlwaysSaysSomething(t *testing.T) {
	got := Version()
	if strings.TrimSpace(got) == "" {
		t.Fatal("Version() is empty — a pod that cannot say what it is")
	}
	if strings.ContainsAny(got, " \t\n\"") {
		// It becomes a Prometheus label value and a log field. A space turns
		// one field into two and a quote ends the label early.
		t.Fatalf("Version() is %q, which does not fit in a label or a log field", got)
	}
}

func TestFullNamesTheProductAndTheToolchain(t *testing.T) {
	got := Full()
	if !strings.HasPrefix(got, "cronos ") {
		t.Fatalf("Full() is %q, and somebody pasting it into an issue should not have to say what it is", got)
	}
	if !strings.Contains(got, Version()) {
		t.Fatalf("Full() is %q and does not contain the version %q", got, Version())
	}
	if !strings.Contains(got, "go1.") {
		t.Fatalf("Full() is %q, with no toolchain in it", got)
	}
}

// A tagged release wins over the commit, which is the whole reason the variable
// exists — and it is set by a linker flag, so nothing else in the build can
// prove it is wired up.
func TestATagBeatsTheCommit(t *testing.T) {
	was := version
	t.Cleanup(func() { version = was })

	version = "1.4.0"
	if got := Version(); got != "1.4.0" {
		t.Fatalf("with a tag set, Version() is %q", got)
	}
	if !strings.HasPrefix(Full(), "cronos 1.4.0 ") {
		t.Fatalf("Full() is %q", Full())
	}
}
