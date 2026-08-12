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
	"log/slog"
	"os"

	"github.com/gsoultan/cronos/internal/platform/boot"
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := boot.Serve(log); err != nil {
		log.Error("cronosd stopped", "err", err)
		os.Exit(1)
	}
}
