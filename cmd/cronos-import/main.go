/*
Command cronos-import turns a directory of JasperReports files into cronos
definitions.

	cronos-import -datasource warehouse -out ./definitions ./jasper

It exists because a reporting estate is the switching cost. Four hundred
`.jrxml` files hold years of decisions about which join is correct, and a team
asked to retype them does not move. What this carries is the meaning — the
query, its parameters, the fields, the grouping, the subtotals and the paper —
and what it cannot carry it says, per file, so an estate can be triaged rather
than trusted.

Nothing is written until -out is given. The default run reads the estate and
prints what would happen, because the first question about a migration tool is
what it will do to four hundred files, and the answer should not require a
backup to discover.

Exit status is 1 when any file was blocked, so a migration can be scripted and
noticed.
*/
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/gsoultan/cronos/internal/adapter/codec/jrxml"
)

func main() {
	var (
		datasource = flag.String("datasource", "warehouse",
			"the cronos DataSource the imported datasets read")
		out = flag.String("out", "",
			"directory to write definitions into; empty reads and reports without writing")
		folder = flag.String("folder", "",
			"catalog folder for the imported reports, e.g. /finance")
		share = flag.Bool("share-datasets", true,
			"when two reports carry the same query, import one dataset and point both at it")
		force = flag.Bool("force", false,
			"overwrite definitions that already exist and differ")
		verbose = flag.Bool("v", false,
			"print every finding, including the cosmetic ones")
	)
	flag.Usage = usage
	flag.Parse()

	if flag.NArg() == 0 {
		usage()
		os.Exit(2)
	}
	run := &importer{
		from:    jrxml.Importer{DataSource: *datasource, Folder: *folder},
		out:     *out,
		share:   *share,
		force:   *force,
		verbose: *verbose,
	}
	if err := run.walk(flag.Args()); err != nil {
		fmt.Fprintln(os.Stderr, "cronos-import:", err)
		os.Exit(2)
	}
	if run.blocked > 0 {
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `usage: cronos-import [flags] <file-or-directory>...

Reads JasperReports .jrxml files and writes cronos Dataset and Report
definitions. Without -out it reports what it would do and writes nothing.

`)
	flag.PrintDefaults()
}

// importer holds one run over an estate.
