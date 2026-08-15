package discord

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/bwmarrin/discordgo"

	"github.com/Phage-Solutions/raider-mate-discord-bot/internal/client"
)

var update = flag.Bool("update", false, "rewrite the golden files")

// A fixed time so the golden files do not change every run. Tuesday 20:00 UTC.
var raidNight = time.Date(2026, time.September, 1, 20, 0, 0, 0, time.UTC)

func intp(n int) *int { return &n }

func summary(name string, roles ...client.Role) *client.CharacterSummary {
	choices := make([]client.RoleChoice, len(roles))
	for i, r := range roles {
		choices[i] = client.RoleChoice{Role: r, Priority: int16(i + 1)}
	}
	return &client.CharacterSummary{ID: name, Name: name, Realm: "Draenor", Roles: choices}
}

func signup(name string, status client.SignupStatus, roles ...client.Role) client.Signup {
	return client.Signup{
		ID:          name,
		CharacterID: name,
		Character:   summary(name, roles...),
		Status:      status,
	}
}

func testEvent(t *testing.T, template client.CompTemplate) client.Event {
	t.Helper()

	encoded, err := json.Marshal(template)
	if err != nil {
		t.Fatalf("encoding template: %v", err)
	}
	difficulty := client.DifficultyHeroic
	return client.Event{
		ID:             "0192f3c8-0000-7000-8000-000000000001",
		Type:           client.EventRaid,
		Title:          "Heroic Nerub-ar Palace",
		StartsAt:       raidNight,
		SignupDeadline: raidNight.Add(-32 * time.Hour),
		CompTemplate:   encoded,
		Difficulty:     &difficulty,
	}
}

// assertGolden serialises the embed and diffs it against a committed file. Run with
// -update to rewrite them, then read the diff before committing it.
func assertGolden(t *testing.T, name string, embed *discordgo.MessageEmbed) {
	t.Helper()

	got, err := json.MarshalIndent(embed, "", "  ")
	if err != nil {
		t.Fatalf("encoding embed: %v", err)
	}
	got = append(got, '\n')

	path := filepath.Join("testdata", name+".json")
	if *update {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatalf("creating testdata: %v", err)
		}
		if err := os.WriteFile(path, got, 0o644); err != nil { //nolint:gosec
			t.Fatalf("writing golden file: %v", err)
		}
		return
	}

	want, err := os.ReadFile(path) //nolint:gosec
	if err != nil {
		t.Fatalf("reading golden file (run go test -update): %v", err)
	}
	if string(got) != string(want) {
		t.Errorf("embed does not match %s\n got: %s\nwant: %s", path, got, want)
	}
}

func TestEventEmbedBeforeALockGroupsBySignupRoles(t *testing.T) {
	view := EventView{
		Event: testEvent(t, client.CompTemplate{Tanks: intp(2), Healers: intp(4), MaxMelee: intp(7), MaxRanged: intp(7)}),
		Signups: []client.Signup{
			signup("Danthrax", client.StatusConfirmed, client.RoleTank, client.RoleMelee),
			signup("Grimwall", client.StatusConfirmed, client.RoleTank),
			signup("Lightbringer", client.StatusConfirmed, client.RoleHealer),
			signup("Mossheart", client.StatusTentative, client.RoleHealer),
			signup("Sunwell", client.StatusDeclined, client.RoleRanged),
		},
	}

	assertGolden(t, "event_unlocked", BuildEventEmbed(view))
}

func TestEventEmbedAfterALockReadsTheCompAndItsBench(t *testing.T) {
	view := EventView{
		Event: testEvent(t, client.CompTemplate{Tanks: intp(2), Healers: intp(4)}),
		Signups: []client.Signup{
			signup("Danthrax", client.StatusConfirmed, client.RoleTank, client.RoleMelee),
			signup("Grimwall", client.StatusConfirmed, client.RoleTank),
			signup("Mossheart", client.StatusConfirmed, client.RoleHealer),
			signup("Thornrend", client.StatusConfirmed, client.RoleMelee),
		},
		Board: &client.Board{
			Name: client.DefaultComp,
			Mode: client.ModeAuto,
			Slots: []client.Assignment{
				{CharacterID: "Danthrax", Character: summary("Danthrax", client.RoleTank, client.RoleMelee), Role: client.RoleTank, SlotIndex: 0, Reason: "TANK: priority 1, main, first signup"},
				{CharacterID: "Grimwall", Character: summary("Grimwall", client.RoleTank), Role: client.RoleTank, SlotIndex: 1, Reason: "TANK: priority 1"},
				{CharacterID: "Mossheart", Character: summary("Mossheart", client.RoleHealer), Role: client.RoleHealer, SlotIndex: 0, Reason: "HEALER: priority 1"},
				{CharacterID: "Thornrend", Character: summary("Thornrend", client.RoleMelee), Role: client.RoleMelee, SlotIndex: 0, IsBench: true, Reason: "no seat left in MDPS"},
			},
			Advisories: []client.Advisory{
				{Role: client.RoleHealer, Message: "1 healer, suggestion for this roster is 4"},
			},
		},
	}

	assertGolden(t, "event_locked", BuildEventEmbed(view))
}

// An empty category must be absent rather than present and zero. A raid with nobody
// tentative should not spend a field saying so.
func TestEventEmbedOmitsEmptyCategories(t *testing.T) {
	view := EventView{
		Event:   testEvent(t, client.CompTemplate{Tanks: intp(2)}),
		Signups: []client.Signup{signup("Grimwall", client.StatusConfirmed, client.RoleTank)},
	}

	for _, field := range BuildEventEmbed(view).Fields {
		if strings.Contains(field.Name, "Tentative") || strings.Contains(field.Name, "Declined") {
			t.Errorf("field %q present, want empty categories omitted", field.Name)
		}
	}
}

func TestFlexRaidersCarryAMarkerAndSingleRoleRaidersDoNot(t *testing.T) {
	// summary ranks the roles in the order given, so tank is this raider's main.
	flex := raiderName(nil, summary("Danthrax", client.RoleTank, client.RoleMelee), client.RoleTank)
	if flex != "Danthrax (offspec melee)" {
		t.Errorf("name = %q, want the other role labelled off-spec", flex)
	}

	pure := raiderName(nil, summary("Grimwall", client.RoleTank), client.RoleTank)
	if pure != "Grimwall" {
		t.Errorf("name = %q, want no marker on a single-role raider", pure)
	}
}

// The label has to flip after a comp lock moves someone. Seated as melee with tank
// ranked first, the useful fact is that they are off-spec and main tank, and an
// unlabelled "tank" said neither.
func TestARaiderSeatedOffSpecShowsTheirMain(t *testing.T) {
	danthrax := summary("Danthrax", client.RoleTank, client.RoleMelee)

	if got := raiderName(nil, danthrax, client.RoleMelee); got != "Danthrax (main tank)" {
		t.Errorf("name = %q, want their main named", got)
	}
	if got := raiderName(nil, danthrax, client.RoleTank); got != "Danthrax (offspec melee)" {
		t.Errorf("name = %q, want the spare roles marked off-spec", got)
	}
}

func TestSeveralOffSpecRolesAreListedTogether(t *testing.T) {
	versatile := summary("Imeya", client.RoleHealer, client.RoleRanged, client.RoleMelee)

	if got := raiderName(nil, versatile, client.RoleHealer); got != "Imeya (offspec ranged/melee)" {
		t.Errorf("name = %q, want every spare role in priority order", got)
	}
}

// The service omits the summary for a character deleted between the signup read and
// the roster read. Rendering "unknown raider" beats inventing a name.
func TestARaiderWithNoSummaryRendersAsUnknown(t *testing.T) {
	if got := raiderName(nil, nil, client.RoleTank); got != "unknown raider" {
		t.Errorf("name = %q, want a placeholder that does not claim to be a name", got)
	}
}

// Discord refuses a field over 1024 characters outright, so a twenty-person list with
// markers has to lose its tail rather than the whole message.
func TestALongRosterTruncatesToACountInsideTheFieldCap(t *testing.T) {
	signups := make([]client.Signup, 0, 40)
	for i := range 40 {
		name := "Verylongraidername" + string(rune('A'+i%26)) + string(rune('a'+i/26))
		signups = append(signups, signup(name, client.StatusConfirmed, client.RoleRanged, client.RoleMelee))
	}

	embed := BuildEventEmbed(EventView{
		Event:   testEvent(t, client.CompTemplate{MaxRanged: intp(7)}),
		Signups: signups,
	})

	var checked bool
	for _, field := range embed.Fields {
		if len(field.Value) > maxFieldValue {
			t.Errorf("field %q is %d characters, want at most %d", field.Name, len(field.Value), maxFieldValue)
		}
		if strings.Contains(field.Name, "Ranged") {
			checked = true
			if !strings.Contains(field.Value, "and ") {
				t.Errorf("field %q does not say how many were dropped: %q", field.Name, field.Value)
			}
		}
	}
	if !checked {
		t.Fatal("no Ranged field, want the long roster rendered")
	}
}

func TestRoleHeadingShowsFilledAgainstWanted(t *testing.T) {
	if got := roleHeading(client.RoleTank, "Tanks", 2, intp(2)); !strings.Contains(got, "(2/2)") {
		t.Errorf("heading = %q, want the count against the requirement", got)
	}
	if got := roleHeading(client.RoleHealer, "Healers", 3, intp(4)); !strings.Contains(got, "(3/4)") {
		t.Errorf("heading = %q, want the gap visible", got)
	}
	if got := roleHeading(client.RoleMelee, "Melee", 5, nil); !strings.Contains(got, "(5)") {
		t.Errorf("heading = %q, want no denominator when the template sets none", got)
	}
	if got := roleHeading(client.RoleTank, "Tanks", 2, intp(2)); !strings.HasPrefix(got, roleGlyphs[client.RoleTank]) {
		t.Errorf("heading = %q, want a glyph to scan for", got)
	}
}

// The gaps are what a raid lead is scanning for, so a role the comp asks for and nobody
// filled must show as 0/2 rather than vanish.
func TestARoleTheCompNeedsShowsEvenWhenEmpty(t *testing.T) {
	view := EventView{
		Event:   testEvent(t, client.CompTemplate{Tanks: intp(2), Healers: intp(4)}),
		Signups: []client.Signup{signup("Grimwall", client.StatusConfirmed, client.RoleTank)},
	}

	var sawHealers bool
	for _, field := range BuildEventEmbed(view).Fields {
		if strings.Contains(field.Name, "Healers") {
			sawHealers = true
			if !strings.Contains(field.Name, "(0/4)") {
				t.Errorf("healer heading = %q, want (0/4)", field.Name)
			}
		}
	}
	if !sawHealers {
		t.Error("no Healers field, want an unfilled requirement to be the obvious thing")
	}
}

// Times must never be formatted bot-side: a guild spanning four countries reads one
// message, and only Discord's own timestamps render per viewer.
func TestEventTimesAreDiscordTimestamps(t *testing.T) {
	embed := BuildEventEmbed(EventView{Event: testEvent(t, client.CompTemplate{})})

	absolute := "<t:" + strconv.FormatInt(raidNight.Unix(), 10) + ":F>"
	if !strings.Contains(embed.Description, absolute) {
		t.Errorf("description = %q, want the absolute timestamp %s", embed.Description, absolute)
	}
	if !strings.Contains(embed.Description, ":R>") {
		t.Errorf("description = %q, want a relative Discord timestamp too", embed.Description)
	}
}

// A raider who confirmed but never ran /character roles has an empty role menu. They
// were previously dropped from every field while the footer still counted them, so the
// embed said "1 signed up" above an empty roster. Whatever else changes, a confirmed
// raider has to appear somewhere.
func TestAConfirmedRaiderWithNoRoleMenuIsStillVisible(t *testing.T) {
	view := EventView{
		Event: testEvent(t, client.CompTemplate{Tanks: intp(2)}),
		Signups: []client.Signup{
			{ID: "s1", CharacterID: "c1", Status: client.StatusConfirmed,
				Character: &client.CharacterSummary{ID: "c1", Name: "Roleless"}},
		},
	}

	embed := BuildEventEmbed(view)

	var found bool
	for _, field := range embed.Fields {
		if strings.Contains(field.Value, "Roleless") {
			found = true
		}
	}
	if !found {
		t.Errorf("Roleless appears in no field, want a confirmed raider visible: %+v", embed.Fields)
	}
}

// A flex raider is one person, not two. Listing them under every playable role would
// make "3/4 healers" a number a raid lead cannot act on.
func TestAFlexRaiderIsListedOnceUnderTheirTopPriorityRole(t *testing.T) {
	view := EventView{
		Event: testEvent(t, client.CompTemplate{Tanks: intp(2), MaxMelee: intp(7)}),
		Signups: []client.Signup{
			signup("Danthrax", client.StatusConfirmed, client.RoleTank, client.RoleMelee),
		},
	}

	var appearances int
	for _, field := range BuildEventEmbed(view).Fields {
		if strings.Contains(field.Value, "Danthrax") {
			appearances++
		}
	}
	if appearances != 1 {
		t.Errorf("Danthrax appears in %d fields, want exactly one", appearances)
	}
}

// Role fields must be inline or Discord stacks them one per row, which is what made
// the roster unreadable next to a bot that lays them out in columns.
func TestRoleFieldsAreInline(t *testing.T) {
	view := EventView{
		Event: testEvent(t, client.CompTemplate{Tanks: intp(2)}),
		Signups: []client.Signup{
			signup("Grimwall", client.StatusConfirmed, client.RoleTank),
		},
	}

	for _, field := range BuildEventEmbed(view).Fields {
		if !field.Inline {
			t.Errorf("field %q is not inline, want columns", field.Name)
		}
	}
}

// Only the configured roles may ping. Without an explicit allow list Discord parses the
// content instead, so an @everyone in a raid title would ping the whole server.
func TestOnlyConfiguredRolesArePermittedToPing(t *testing.T) {
	allowed := allowedRoleMentions([]string{"781", "799"})

	if len(allowed.Parse) != 0 {
		t.Errorf("parse = %v, want it empty so nothing is inferred from the content", allowed.Parse)
	}
	if len(allowed.Roles) != 2 {
		t.Errorf("roles = %v, want exactly the configured two", allowed.Roles)
	}
}

func TestAGuildPingingNobodyPermitsNothing(t *testing.T) {
	allowed := allowedRoleMentions(nil)

	if len(allowed.Parse) != 0 || len(allowed.Roles) != 0 {
		t.Errorf("allowed = %+v, want nothing permitted to ping", allowed)
	}
}

func TestMentionListRendersRolePings(t *testing.T) {
	if got := mentionList([]string{"781", "799"}); got != "<@&781> <@&799>" {
		t.Errorf("mentions = %q, want role pings", got)
	}
	if got := mentionList(nil); got != "" {
		t.Errorf("mentions = %q, want an empty content line", got)
	}
}

// Item level is what a raid lead reads off a roster, but only once the worker's
// Raider.IO sync has filled it in. A fresh registration shows a name and nothing else,
// rather than claiming a gear level of zero.
func TestItemLevelAppearsOnlyOnceSynced(t *testing.T) {
	ilvl := 639.4
	geared := summary("Danthrax", client.RoleTank)
	geared.Ilvl = &ilvl

	if got := raiderName(nil, geared, client.RoleTank); got != "Danthrax `639`" {
		t.Errorf("name = %q, want the item level rounded and shown", got)
	}
	if got := raiderName(nil, summary("Freshling", client.RoleTank), client.RoleTank); got != "Freshling" {
		t.Errorf("name = %q, want no gear level before the sync has run", got)
	}
}

// Discord puts three inline fields on a row, so four role columns wrap as three then
// one, and the stray fourth sits under the first column looking like part of it. Each
// section is padded to a whole row so the next one starts clean.
func TestSectionsArePaddedToWholeRows(t *testing.T) {
	view := EventView{
		Event: testEvent(t, client.CompTemplate{Tanks: intp(2), Healers: intp(4), MaxMelee: intp(7), MaxRanged: intp(7)}),
		Signups: []client.Signup{
			signup("Grimwall", client.StatusConfirmed, client.RoleTank),
			signup("Sunwell", client.StatusDeclined, client.RoleRanged),
		},
	}

	fields := BuildEventEmbed(view).Fields

	// Four role columns plus two spacers, then one status column plus two spacers.
	if len(fields)%3 != 0 {
		t.Errorf("%d fields, want a whole number of rows", len(fields))
	}

	var blanks int
	for _, f := range fields {
		if f.Name == "\u200b" {
			blanks++
		}
	}
	if blanks != 4 {
		t.Errorf("%d spacer fields, want 4: two to finish each section", blanks)
	}
}

// A Mythic+ group is told apart from a raid by an explicit line: a raid usually says so
// in its title, a dungeon does not, and neither carries a difficulty.
func TestADungeonEmbedNamesItsTypeAndHasNoDifficulty(t *testing.T) {
	event := testEvent(t, client.CompTemplate{})
	event.Type = client.EventMythicPlus
	event.Title = "+12 Ara-Kara"
	event.Difficulty = nil

	description := BuildEventEmbed(EventView{Event: event}).Description

	if !strings.Contains(description, "Mythic+") {
		t.Errorf("description = %q, want the group type named", description)
	}
	if strings.Contains(description, "Difficulty") {
		t.Errorf("description = %q, want no difficulty on a dungeon", description)
	}
}
