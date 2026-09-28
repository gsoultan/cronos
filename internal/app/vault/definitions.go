package vault

import "github.com/gsoultan/cronos/internal/core/definition"

// Definitions are the running view of a project, read to say which secrets it
// uses and to find the datasources a changed one belongs to.
type Definitions interface {
	DataSources() []definition.DataSource
	Reports() []definition.Report
}
