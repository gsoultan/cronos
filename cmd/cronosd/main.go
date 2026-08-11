// Command cronosd is the Business Source License build of the cronos server.
//
// It must not import github.com/gsoultan/cronos/ee, directly or transitively.
package main

import (
	"fmt"

	"github.com/gsoultan/cronos/internal/extension"
)

func main() {
	fmt.Printf("cronos (community) auth=%s audit=%s\n",
		extension.Auth().Name(), extension.Audit().Name())
}
