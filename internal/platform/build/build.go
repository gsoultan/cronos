// Package build says what this binary is.
//
// An operator holding a pod that is misbehaving needs to be able to answer
// "which build is this", and until this there was nothing to answer with: no
// flag, no log line, no metric, no VERSION file. During a rolling deploy —
// which is the one time the question is guaranteed to be asked — a fleet runs
// two versions at once and nothing distinguished them.
package build

import (
	"fmt"
	"runtime/debug"
	"strings"
)

/*
version is set at link time for a tagged release:

	go build -ldflags "-X github.com/gsoultan/cronos/internal/platform/build.version=1.4.0"

Empty otherwise, which is most builds, and the commit is used instead. Nothing
has to remember to pass anything: `go build` records the revision by itself.
*/
var version string

// Version is what to print, log and label a metric with.
//
// A tag where there is one, otherwise the commit — twelve characters of it,
// which is enough to find and short enough to read out over a call. `+dirty`
// where the tree had uncommitted changes, because a build from a working copy
// is not the commit it claims to be and that is exactly the case somebody is
// trying to explain when they ask.
func Version() string {
	if version != "" {
		return version
	}

	info, ok := debug.ReadBuildInfo()
	if !ok {
		// `go run`, or a binary built with the VCS stamp turned off. Saying so
		// beats a blank, which reads as a bug in this function.
		return "unknown"
	}

	var revision, modified string
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			revision = s.Value
		case "vcs.modified":
			modified = s.Value
		}
	}
	if revision == "" {
		return "unknown"
	}
	if len(revision) > 12 {
		revision = revision[:12]
	}
	if modified == "true" {
		return revision + "+dirty"
	}
	return revision
}

// Full is Version with the toolchain and platform, for a `-version` flag.
//
// More than a log line wants and exactly what somebody pastes into an issue.
func Full() string {
	info, _ := debug.ReadBuildInfo()
	parts := []string{"cronos " + Version()}
	if info != nil {
		parts = append(parts, info.GoVersion)
		for _, s := range info.Settings {
			if s.Key == "GOOS" || s.Key == "GOARCH" {
				parts = append(parts, s.Value)
			}
		}
	}
	return strings.Join(parts, " ")
}

// String is Full, so a %v of this package's answer reads properly.
func String() string { return fmt.Sprint(Full()) }
