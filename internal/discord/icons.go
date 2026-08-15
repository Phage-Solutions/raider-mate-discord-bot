package discord

import (
	"context"
	"fmt"
	"strings"

	"github.com/bwmarrin/discordgo"
)

// IconSet maps an emoji name to the markup that renders it. Two kinds of key live in
// here: a class on its own ("warrior"), and a spec qualified by its class
// ("mage_frost").
//
// Specs have to carry their class because spec names are not unique. Frost is a Mage
// and a Death Knight, Holy is a Paladin and a Priest, Restoration is a Druid and a
// Shaman. Keying on the spec alone would give a Frost Death Knight a Mage icon.
type IconSet map[string]string

// Markup returns the best icon available for a character: their spec if one is
// uploaded, otherwise their class, otherwise nothing.
//
// The fallback is what makes a partial upload useful. A guild that has only put up
// thirteen class icons gets class icons, rather than nothing until all thirty-nine
// specs exist.
func (s IconSet) Markup(class, spec *string) string {
	if len(s) == 0 || class == nil {
		return ""
	}

	if spec != nil {
		if markup, ok := s[iconKey(*class, *spec)]; ok {
			return markup + " "
		}
	}
	if markup, ok := s[iconKey(*class, "")]; ok {
		return markup + " "
	}
	return ""
}

// iconKey normalises what Raider.IO reports into an emoji name. Discord allows letters,
// digits and underscores, so "Beast Mastery" on a "Hunter" becomes
// hunter_beastmastery.
func iconKey(class, spec string) string {
	name := normaliseIconName(class)
	if spec != "" {
		name += "_" + normaliseIconName(spec)
	}
	return name
}

func normaliseIconName(raw string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(raw) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// loadIcons reads the application's own emoji, so uploading one called warrior or
// mage_frost is the whole configuration step. Application emoji belong to the app
// rather than a guild, which is what makes them usable in every server the bot is in.
//
// A failure here is not a startup failure. Icons are decoration, and a bot that refused
// to start because an emoji list did not load would be trading a working roster for a
// prettier one.
func loadIcons(ctx context.Context, session *discordgo.Session, appID string) IconSet {
	emojis, err := session.ApplicationEmojis(appID, discordgo.WithContext(ctx))
	if err != nil {
		return IconSet{}
	}

	icons := make(IconSet, len(emojis))
	for _, emoji := range emojis {
		icons[strings.ToLower(emoji.Name)] = emojiMarkup(emoji)
	}
	return icons
}

func emojiMarkup(emoji *discordgo.Emoji) string {
	if emoji.Animated {
		return fmt.Sprintf("<a:%s:%s>", emoji.Name, emoji.ID)
	}
	return fmt.Sprintf("<:%s:%s>", emoji.Name, emoji.ID)
}
