package main

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
)

// dotEnvPath is where local configuration lives, relative to the working directory.
const dotEnvPath = ".env"

// loadDotEnv reads KEY=value pairs from path into the process environment.
//
// The service leaves this to its Makefile and reads no dotenv itself. This repo does
// read one, because the bot is run from an IDE as often as from a terminal, and a run
// configuration that depends on a marketplace plugin to pass the Discord token is a
// prerequisite that gets missed. The alternative, putting the token in the run
// configuration, would commit it: .run/ is tracked and .env is not.
//
// Two rules make it safe to call unconditionally. A variable already present in the
// environment always wins, so a container or CI passing real configuration is never
// overridden by a stray file. A missing file is not an error, since having none is the
// normal case everywhere except a developer's machine.
func loadDotEnv(path string) error {
	file, err := os.Open(path) //nolint:gosec
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("opening %s: %w", path, err)
	}
	defer file.Close() //nolint:errcheck

	scanner := bufio.NewScanner(file)
	for line := 1; scanner.Scan(); line++ {
		key, value, err := parseDotEnvLine(scanner.Text())
		if err != nil {
			return fmt.Errorf("%s:%d: %w", path, line, err)
		}
		if key == "" {
			continue
		}
		if _, alreadySet := os.LookupEnv(key); alreadySet {
			continue
		}
		if err := os.Setenv(key, value); err != nil {
			return fmt.Errorf("setting %s: %w", key, err)
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("reading %s: %w", path, err)
	}
	return nil
}

// parseDotEnvLine reads one line. A blank line or a comment returns an empty key and
// no error, which the caller skips.
//
// Inline comments are not supported: a value runs to the end of the line. Discord
// tokens and URLs contain no spaces, and treating a stray # as a comment would corrupt
// a password more often than it would tidy a file.
func parseDotEnvLine(raw string) (key, value string, err error) {
	line := strings.TrimSpace(raw)
	if line == "" || strings.HasPrefix(line, "#") {
		return "", "", nil
	}
	line = strings.TrimPrefix(line, "export ")

	key, value, found := strings.Cut(line, "=")
	if !found {
		return "", "", fmt.Errorf("no = in %q", raw)
	}

	key = strings.TrimSpace(key)
	if key == "" {
		return "", "", fmt.Errorf("no name before = in %q", raw)
	}

	return key, unquote(strings.TrimSpace(value)), nil
}

// unquote strips one layer of matching surrounding quotes, so a value with trailing
// spaces can be written deliberately.
func unquote(value string) string {
	if len(value) < 2 {
		return value
	}
	quote := value[0]
	if (quote == '"' || quote == '\'') && value[len(value)-1] == quote {
		return value[1 : len(value)-1]
	}
	return value
}
