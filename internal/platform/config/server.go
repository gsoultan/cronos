package config

import (
	"fmt"
	"os"
	"strings"
)

// Server is everything cronosd needs to start.
type Server struct {
	Addr string
	// Definitions is a directory of YAML.
	Definitions string
	// Driver and DSN say where rows live. "sqlite" and a path is the
	// development answer; "postgres" and a URL is the deployed one.
	Driver string
	DSN    string
	// SigningKey signs embed tokens. No default — a default signing key is a
	// shared trust root across every deployment that forgot to set one.
	SigningKey []byte
	// Origins are the host pages allowed to embed. No wildcard; see api.CORS.
	Origins []string
	// Seed is a .sql file applied at startup. Development only: it exists so
	// an in-memory database has something in it, and a deployment that points
	// this at a real DSN is running DDL on every restart.
	Seed string
}

// Load reads the environment, filling in the defaults that are safe to have.
func Load() (Server, error) {
	s := Server{
		Addr:        env("CRONOS_ADDR", ":8787"),
		Definitions: env("CRONOS_DEFINITIONS", "examples"),
		Driver:      env("CRONOS_DRIVER", "sqlite"),
		DSN:         env("CRONOS_DSN", ":memory:"),
		SigningKey:  []byte(os.Getenv("CRONOS_SIGNING_KEY")),
		Seed:        os.Getenv("CRONOS_SEED"),
	}
	if origins := os.Getenv("CRONOS_ORIGINS"); origins != "" {
		s.Origins = strings.Split(origins, ",")
	}
	if len(s.SigningKey) == 0 {
		// Refused rather than generated. A key generated at startup silently
		// invalidates every token on restart, and one baked in as a default is
		// the same key everyone else's deployment is using.
		return Server{}, fmt.Errorf("config: CRONOS_SIGNING_KEY is required")
	}
	return s, nil
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
