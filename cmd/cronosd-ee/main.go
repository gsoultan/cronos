/*
Command cronosd-ee is the Enterprise Edition build of the cronos server.

The same server as cmd/cronosd — the same assembly, from the same package —
with ee/ imported for its side effects, so its implementations replace the
defaults at init time before anything reads a seam.

It printed two names and exited before this, which made every seam in
internal/extension a thing nobody could run.
*/
package main

import (
	"log/slog"
	"os"

	"github.com/gsoultan/cronos/internal/platform/boot"

	_ "github.com/gsoultan/cronos/ee"
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := boot.Serve(log); err != nil {
		log.Error("cronosd-ee stopped", "err", err)
		os.Exit(1)
	}
}
