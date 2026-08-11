// Package config reads the server's settings from the environment.
//
// Environment rather than a file, because the two settings that matter — the
// signing key and the database URL — are secrets, and a secret in a file is a
// secret in a backup.
package config
