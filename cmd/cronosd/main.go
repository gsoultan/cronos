/*
Command cronosd is the Business Source License build of the cronos server.

A main and nothing else: the assembly lives in internal/platform/boot, so the
enterprise binary can run the same server rather than reimplement it. What
differs between the two is one import.

It must not reach github.com/gsoultan/cronos/ee, directly or transitively —
scripts/check-license-boundary.sh checks that against the actual build graph.
*/
package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"

	"github.com/gsoultan/cronos/internal/platform/boot"
	"github.com/gsoultan/cronos/internal/platform/build"
)

func main() {
	/*
	   `cronosd -version`, before anything else.

	   Ahead of reading configuration, because the moment somebody asks which
	   build this is, is usually the moment it will not start — and answering
	   "CRONOS_SIGNING_KEY is required" to a question about a version is how
	   somebody ends up guessing from an image digest.
	*/
	wanted := flag.Bool("version", false, "print the build and exit")
	flag.Parse()
	if *wanted {
		fmt.Println(build.Full())
		return
	}

	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := boot.Serve(log); err != nil {
		log.Error("cronosd stopped", "err", err)
		os.Exit(1)
	}
}
