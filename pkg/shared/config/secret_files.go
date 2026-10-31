package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// SecretFilesDir is where compose mounts a service's `secrets:`.
const SecretFilesDir = "/run/secrets"

var envName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// LoadSecretFiles sets, for each file in dir named like an environment
// variable, that variable to the file's content (trailing newlines trimmed),
// unless the variable is already set. It returns the names it set.
//
// Secrets reach the containers as files so that `docker inspect` does not
// print them; this turns them back into the environment every os.Getenv
// caller already reads, so none of those callers change. A missing dir is not
// an error: local runs have none. An unreadable file is, because booting
// without a secret the deployment provided fails later and less legibly.
func LoadSecretFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read secret dir %s: %w", dir, err)
	}
	var set []string
	for _, e := range entries {
		name := e.Name()
		if !envName.MatchString(name) {
			continue
		}
		path := filepath.Join(dir, name)
		info, err := os.Stat(path)
		if err != nil {
			return set, fmt.Errorf("stat secret %s: %w", name, err)
		}
		if info.IsDir() {
			continue
		}
		if _, ok := os.LookupEnv(name); ok {
			continue
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return set, fmt.Errorf("read secret %s: %w", name, err)
		}
		if err := os.Setenv(name, strings.TrimRight(string(b), "\r\n")); err != nil {
			return set, fmt.Errorf("set %s: %w", name, err)
		}
		set = append(set, name)
	}
	return set, nil
}
