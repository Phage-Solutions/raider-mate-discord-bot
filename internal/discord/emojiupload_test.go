package discord

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDataURIUsesTheGivenMIMEType(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "warrior.webp")
	if err := os.WriteFile(path, []byte("not a real image, just bytes"), 0o600); err != nil {
		t.Fatalf("writing test file: %v", err)
	}

	uri, err := dataURI(path, emojiMIME[".webp"])
	if err != nil {
		t.Fatalf("dataURI: %v", err)
	}
	if !strings.HasPrefix(uri, "data:image/webp;base64,") {
		t.Errorf("uri = %s, want it to start with the webp data URI prefix", uri)
	}
}

func TestDataURIRejectsFilesOverTheEmojiLimit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "warrior.png")
	if err := os.WriteFile(path, make([]byte, 256*1024+1), 0o600); err != nil {
		t.Fatalf("writing test file: %v", err)
	}

	if _, err := dataURI(path, emojiMIME[".png"]); err == nil {
		t.Error("dataURI accepted a file over 256KB, want it refused")
	}
}

func TestEmojiNamesMatchWhatTheIconLookupAsksFor(t *testing.T) {
	for _, name := range []string{"warrior", "mage_frost", "deathknight_unholy", "hunter_beastmastery"} {
		if err := validEmojiName(name); err != nil {
			t.Errorf("validEmojiName(%q) = %v, want it accepted", name, err)
		}
	}
}

// A hyphen or a space is the failure worth catching. IconSet keys strip everything that
// is not a letter or a digit, so mage-frost.png would upload under a name nothing ever
// looks up: no error, no icon, and nothing to suggest why.
func TestFilenamesTheLookupWouldNeverAskForAreRejected(t *testing.T) {
	for _, name := range []string{"mage-frost", "mage frost", "mage.frost", "mage+frost"} {
		if err := validEmojiName(name); err == nil {
			t.Errorf("validEmojiName(%q) = nil, want it refused before upload", name)
		}
	}
}

func TestEmojiNameLengthIsBounded(t *testing.T) {
	if err := validEmojiName("a"); err == nil {
		t.Error("a one-character name was accepted, want Discord's two-character minimum enforced")
	}

	long := ""
	for range maxEmojiNameLength + 1 {
		long += "a"
	}
	if err := validEmojiName(long); err == nil {
		t.Errorf("a %d-character name was accepted, want Discord's %d maximum enforced",
			len(long), maxEmojiNameLength)
	}
}

// Every filename the README tells people to use has to survive validation, or the
// documentation and the uploader disagree.
func TestTheDocumentedSpecNamesAreAllValid(t *testing.T) {
	for _, name := range []string{
		"deathknight", "deathknight_blood", "deathknight_frost", "deathknight_unholy",
		"demonhunter_vengeance", "druid_restoration", "evoker_augmentation",
		"hunter_beastmastery", "monk_brewmaster", "paladin_retribution",
		"priest_discipline", "rogue_assassination", "shaman_enhancement",
		"warlock_demonology", "warrior_protection",
	} {
		if err := validEmojiName(name); err != nil {
			t.Errorf("documented name %q is invalid: %v", name, err)
		}
	}
}
