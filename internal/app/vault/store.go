package vault

import "context"

// Store is where sealed secrets are kept, scoped by organisation and project
// on every call.
type Store interface {
	Sealed(ctx context.Context, org, project, name string) ([]byte, bool, error)
	// SecretsOf lists a project's secrets without their values.
	SecretsOf(ctx context.Context, org, project string) ([]Stored, error)
	PutSecret(ctx context.Context, s Stored) error
	DeleteSecret(ctx context.Context, org, project, name string) (bool, error)
}
