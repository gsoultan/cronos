package secret

/*
Only answers for the names it was given, and for nothing else.

A deployment serving several projects has one environment and one secrets
directory, and every project's definitions used to resolve against all of it.
That made each deployment secret readable by every project: an editor in one
could publish a datasource whose DSN points at a host they run and names
another project's warehouse password, and the process would resolve it and
send it there on the first connection.

So a project of several sees the deployment's secrets only by name, from the
list its operator shared (CRONOS_SHARED_SECRETS) — a Mapbox token every project
uses, say. Everything else a project needs is its own: stored in the project,
or a file under the secrets directory's org/project.
*/
type Only struct {
	Names map[string]bool
	From  Resolver
}

// Secret answers for a shared name and refuses the rest.
func (o Only) Secret(name string) (string, bool) {
	if !o.Names[name] || o.From == nil {
		return "", false
	}
	return o.From.Secret(name)
}
