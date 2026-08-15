package discord

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/bwmarrin/discordgo"
)

// UploadIcons publishes every PNG in dir as an application emoji named after its file,
// so warrior.png becomes :warrior: and mage_frost.png becomes :mage_frost:.
//
// Application emoji belong to the app rather than a guild, so one upload covers every
// server the bot is in, and a self-hoster running their own application uploads their
// own set. Re-running replaces what is already there, which is what makes this safe to
// run again after changing one icon.
func (b *Bot) UploadIcons(ctx context.Context, dir string) error {
	if err := b.session.Open(); err != nil {
		return fmt.Errorf("opening gateway: %w", err)
	}
	defer b.Stop()

	appID := b.appID()
	if appID == "" {
		return fmt.Errorf("could not determine the application id")
	}

	existing, err := b.session.ApplicationEmojis(appID, discordgo.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("listing application emojis: %w", err)
	}
	byName := make(map[string]*discordgo.Emoji, len(existing))
	for _, emoji := range existing {
		byName[strings.ToLower(emoji.Name)] = emoji
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("reading %s: %w", dir, err)
	}

	var uploaded int
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".png") {
			continue
		}

		name := strings.ToLower(strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name())))
		if err := validEmojiName(name); err != nil {
			return fmt.Errorf("%s: %w", entry.Name(), err)
		}

		image, err := dataURI(filepath.Join(dir, entry.Name()))
		if err != nil {
			return err
		}

		// Replacing rather than editing: an edit can only rename, so a changed icon has
		// to go up as a new emoji.
		if old, ok := byName[name]; ok {
			if err := b.session.ApplicationEmojiDelete(appID, old.ID, discordgo.WithContext(ctx)); err != nil {
				return fmt.Errorf("replacing %s: %w", name, err)
			}
		}

		if _, err := b.session.ApplicationEmojiCreate(appID, &discordgo.EmojiParams{
			Name: name, Image: image,
		}, discordgo.WithContext(ctx)); err != nil {
			return fmt.Errorf("uploading %s: %w", name, err)
		}

		b.logger.InfoContext(ctx, "uploaded icon", "name", name)
		uploaded++
	}

	if uploaded == 0 {
		return fmt.Errorf("no .png files in %s, see the README there for the filenames", dir)
	}
	b.logger.InfoContext(ctx, "icons uploaded", "count", uploaded)
	return nil
}

// dataURI reads an image into the base64 form Discord's emoji endpoint expects.
func dataURI(path string) (string, error) {
	raw, err := os.ReadFile(path) //nolint:gosec
	if err != nil {
		return "", fmt.Errorf("reading %s: %w", path, err)
	}
	// Discord caps emoji at 256KB and rejects anything larger with a generic error, so
	// say which file is the problem rather than letting the API be vague about it.
	const maxEmojiBytes = 256 * 1024
	if len(raw) > maxEmojiBytes {
		return "", fmt.Errorf("%s is %dKB, over Discord's 256KB emoji limit", path, len(raw)/1024)
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(raw), nil
}

// Discord's rules for an emoji name.
const (
	minEmojiNameLength = 2
	maxEmojiNameLength = 32
)

// validEmojiName rejects a filename Discord would refuse, or worse, accept in a form the
// icon lookup will never ask for.
//
// The second case is the dangerous one. IconSet keys are built by stripping everything
// that is not a letter or a digit and joining class to spec with an underscore, so
// mage-frost.png would upload under a name nothing ever looks up: no error, no icon, and
// nothing to suggest why.
func validEmojiName(name string) error {
	if len(name) < minEmojiNameLength || len(name) > maxEmojiNameLength {
		return fmt.Errorf("emoji names are %d to %d characters, this one is %d",
			minEmojiNameLength, maxEmojiNameLength, len(name))
	}

	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			continue
		}
		return fmt.Errorf("only letters, digits and underscores are allowed, found %q; "+
			"a spec is its class, an underscore, then the spec, as in mage_frost", r)
	}
	return nil
}
