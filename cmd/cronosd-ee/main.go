// Command cronosd-ee is the Enterprise Edition build of the cronos server: the
// same core, with ee/ imported for side effects so its implementations replace
// the defaults at init time.
package main

import (
	"fmt"

	"github.com/gsoultan/cronos/internal/extension"

	_ "github.com/gsoultan/cronos/ee"
)

func main() {
	fmt.Printf("cronos (enterprise) auth=%s audit=%s\n",
		extension.Auth().Name(), extension.Audit().Name())
}
