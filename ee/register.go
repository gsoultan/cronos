package ee

import (
	"os"

	"github.com/gsoultan/cronos/ee/audit"
	"github.com/gsoultan/cronos/internal/extension"
)

func init() {
	extension.RegisterAuditSink(audit.NewJSONL(os.Stderr))
}
