// Package yaml reads definitions from the file format authors write.
//
// An adapter, not core: the domain types know nothing about how they are
// stored, so the same Dataset can arrive from a file, a database row or an API
// body without the validation rules caring which.
//
// The envelope is Kubernetes-shaped — apiVersion, kind, metadata, spec —
// because report definitions live in the same repositories as the deployment
// manifests beside them, and an engineer who has seen one has seen this.
package yaml
