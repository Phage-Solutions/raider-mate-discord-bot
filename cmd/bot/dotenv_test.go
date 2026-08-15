package main

import (
	"os"
	"path/filepath"
	"testing"
)

// writeDotEnv puts a .env in a temp directory and returns its path.
func writeDotEnv(t *testing.T, contents string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("writing test .env: %v", err)
	}
	return path
}

// unsetEnv makes a variable genuinely absent for the duration of one test. There is no
// t.Unsetenv, so t.Setenv is used first purely to register the restore.
func unsetEnv(t *testing.T, key string) {
	t.Helper()

	t.Setenv(key, "")
	if err := os.Unsetenv(key); err != nil {
		t.Fatalf("unsetting %s: %v", key, err)
	}
}

func TestLoadDotEnvSetsValues(t *testing.T) {
	path := writeDotEnv(t, "DISCORD_TOKEN=abc123\nSERVICE_BASE_URL=http://localhost:8080\n")
	unsetEnv(t, "DISCORD_TOKEN")

	if err := loadDotEnv(path); err != nil {
		t.Fatalf("loading: %v", err)
	}

	if got := os.Getenv("DISCORD_TOKEN"); got != "abc123" {
		t.Errorf("DISCORD_TOKEN = %q, want abc123", got)
	}
}

// A container passes real configuration in the environment. A .env that got copied
// into the image must not quietly win over it.
func TestARealEnvironmentVariableIsNotOverridden(t *testing.T) {
	path := writeDotEnv(t, "DISCORD_TOKEN=from-file\n")
	t.Setenv("DISCORD_TOKEN", "from-environment")

	if err := loadDotEnv(path); err != nil {
		t.Fatalf("loading: %v", err)
	}

	if got := os.Getenv("DISCORD_TOKEN"); got != "from-environment" {
		t.Errorf("DISCORD_TOKEN = %q, want the environment to win", got)
	}
}

// Having no .env is the normal case in a container, so it cannot be a startup failure.
func TestAMissingFileIsNotAnError(t *testing.T) {
	if err := loadDotEnv(filepath.Join(t.TempDir(), "absent")); err != nil {
		t.Errorf("error = %v, want a missing file to be fine", err)
	}
}

func TestCommentsAndBlankLinesAreSkipped(t *testing.T) {
	path := writeDotEnv(t, "# a comment\n\n   \n# another\nLOG_LEVEL=debug\n")
	unsetEnv(t, "LOG_LEVEL")

	if err := loadDotEnv(path); err != nil {
		t.Fatalf("loading: %v", err)
	}

	if got := os.Getenv("LOG_LEVEL"); got != "debug" {
		t.Errorf("LOG_LEVEL = %q, want debug", got)
	}
}

// The shipped .env.example leaves DISCORD_DEV_GUILD_ID empty on purpose: empty means
// register globally, and that has to survive the round trip as empty rather than
// becoming a parse error.
func TestAnEmptyValueIsKept(t *testing.T) {
	path := writeDotEnv(t, "DISCORD_DEV_GUILD_ID=\n")
	unsetEnv(t, "DISCORD_DEV_GUILD_ID")

	if err := loadDotEnv(path); err != nil {
		t.Fatalf("loading: %v", err)
	}

	value, set := os.LookupEnv("DISCORD_DEV_GUILD_ID")
	if !set || value != "" {
		t.Errorf("value = %q set = %v, want an empty value to be set", value, set)
	}
}

func TestSurroundingQuotesAreStripped(t *testing.T) {
	for name, line := range map[string]string{
		"double": `SERVICE_API_KEY="quoted key"`,
		"single": `SERVICE_API_KEY='quoted key'`,
	} {
		t.Run(name, func(t *testing.T) {
			unsetEnv(t, "SERVICE_API_KEY")
			if err := loadDotEnv(writeDotEnv(t, line+"\n")); err != nil {
				t.Fatalf("loading: %v", err)
			}
			if got := os.Getenv("SERVICE_API_KEY"); got != "quoted key" {
				t.Errorf("SERVICE_API_KEY = %q, want the quotes stripped", got)
			}
		})
	}
}

// A # inside a value is part of the value. Treating it as a comment would corrupt a
// password more often than it would tidy a file.
func TestAHashInsideAValueIsNotAComment(t *testing.T) {
	unsetEnv(t, "SERVICE_API_KEY")
	if err := loadDotEnv(writeDotEnv(t, "SERVICE_API_KEY=p#ss word\n")); err != nil {
		t.Fatalf("loading: %v", err)
	}

	if got := os.Getenv("SERVICE_API_KEY"); got != "p#ss word" {
		t.Errorf("SERVICE_API_KEY = %q, want the whole value", got)
	}
}

func TestExportPrefixIsAccepted(t *testing.T) {
	unsetEnv(t, "LOG_LEVEL")
	if err := loadDotEnv(writeDotEnv(t, "export LOG_LEVEL=warn\n")); err != nil {
		t.Fatalf("loading: %v", err)
	}

	if got := os.Getenv("LOG_LEVEL"); got != "warn" {
		t.Errorf("LOG_LEVEL = %q, want warn", got)
	}
}

// A line with no = is a typo, and silently ignoring it would mean starting with
// configuration the author believed they had set.
func TestALineWithNoEqualsIsReported(t *testing.T) {
	err := loadDotEnv(writeDotEnv(t, "DISCORD_TOKEN abc123\n"))
	if err == nil {
		t.Fatal("error = nil, want a malformed line reported")
	}
}
