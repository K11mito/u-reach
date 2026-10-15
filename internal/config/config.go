// Package config loads .env into the process environment and reads settings.
package config

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// Load reads KEY=VALUE lines from path into the environment. Variables that
// are already set win, so `FOO=x go run ...` overrides .env. A missing file is
// not an error.
func Load(path string) error {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(strings.TrimPrefix(k, "export "))
		v = strings.Trim(strings.TrimSpace(v), `"'`)
		if _, set := os.LookupEnv(k); !set {
			os.Setenv(k, v)
		}
	}
	return sc.Err()
}

// Get returns the env var or def when it is unset or empty.
func Get(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// Must returns the env var or exits with a message naming it.
func Must(key string) string {
	v := os.Getenv(key)
	if v == "" {
		fmt.Fprintf(os.Stderr, "missing %s: add it to .env (see .env.example)\n", key)
		os.Exit(1)
	}
	return v
}
