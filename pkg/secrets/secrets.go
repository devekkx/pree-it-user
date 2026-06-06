package secrets

import (
	"fmt"
	"os"
	"strings"
)

func MustRead(name string) string {
	val, err := Read(name)
	if err != nil {
		panic(fmt.Sprintf("secrets.MustRead(%q): %v", name, err))
	}
	return val
}

func Read(name string) (string, error) {
	upper := strings.ToUpper(name)

	// 1. Plain env var set by entrypoint
	if val := strings.TrimSpace(os.Getenv(upper)); val != "" {
		return val, nil
	}

	// 2. Explicit file path override
	if path := os.Getenv(upper + "_FILE"); path != "" {
		return readFile(path, name)
	}

	// 3. Docker default mount path
	return readFile("/run/secrets/"+name, name)
}

func readFile(path, name string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read secret file %q for %q: %w", path, name, err)
	}
	val := strings.TrimSpace(string(data))
	if val == "" {
		return "", fmt.Errorf("secret file %q for %q is empty", path, name)
	}
	return val, nil
}
