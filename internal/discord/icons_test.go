package discord

import "testing"

func strp(s string) *string { return &s }

// Spec names are not unique across classes. Frost is a Mage and a Death Knight, so a
// key that ignored the class would put a Mage icon on a Death Knight.
func TestASpecIconIsKeyedByItsClass(t *testing.T) {
	icons := IconSet{
		"mage_frost":        "<:mage_frost:1>",
		"deathknight_frost": "<:deathknight_frost:2>",
	}

	if got := icons.Markup(strp("Mage"), strp("Frost")); got != "<:mage_frost:1> " {
		t.Errorf("mage markup = %q, want the mage icon", got)
	}
	if got := icons.Markup(strp("Death Knight"), strp("Frost")); got != "<:deathknight_frost:2> " {
		t.Errorf("death knight markup = %q, want the death knight icon", got)
	}
}

// A guild that uploaded thirteen class icons should get class icons, not nothing until
// all thirty-nine specs exist.
func TestASpecWithNoIconFallsBackToItsClass(t *testing.T) {
	icons := IconSet{"hunter": "<:hunter:3>"}

	if got := icons.Markup(strp("Hunter"), strp("Beast Mastery")); got != "<:hunter:3> " {
		t.Errorf("markup = %q, want the class icon as a fallback", got)
	}
}

// Spec is nil until the worker's Raider.IO sync has run, and a guild may have uploaded
// nothing at all. Both render as a bare name rather than as a broken emoji.
func TestMissingIconsRenderAsNothing(t *testing.T) {
	icons := IconSet{"hunter": "<:hunter:3>"}

	if got := icons.Markup(strp("Hunter"), nil); got != "<:hunter:3> " {
		t.Errorf("markup = %q, want the class icon when the spec is unknown", got)
	}
	if got := icons.Markup(strp("Warlock"), strp("Affliction")); got != "" {
		t.Errorf("markup = %q, want nothing when neither is uploaded", got)
	}
	if got := (IconSet{}).Markup(strp("Hunter"), strp("Marksmanship")); got != "" {
		t.Errorf("markup = %q, want nothing from an empty set", got)
	}
	if got := icons.Markup(nil, nil); got != "" {
		t.Errorf("markup = %q, want nothing for an unsynced character", got)
	}
}

// Discord emoji names allow letters, digits and underscores, so what Raider.IO reports
// has to be flattened to that.
func TestIconKeysAreFlattenedToEmojiNames(t *testing.T) {
	for _, tc := range []struct{ class, spec, want string }{
		{"Death Knight", "Frost", "deathknight_frost"},
		{"Hunter", "Beast Mastery", "hunter_beastmastery"},
		{"Demon Hunter", "Havoc", "demonhunter_havoc"},
		{"Warrior", "", "warrior"},
	} {
		if got := iconKey(tc.class, tc.spec); got != tc.want {
			t.Errorf("iconKey(%q, %q) = %q, want %q", tc.class, tc.spec, got, tc.want)
		}
	}
}
