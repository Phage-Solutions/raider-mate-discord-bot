package discord

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/bwmarrin/discordgo"

	"github.com/Phage-Solutions/raider-mate-discord-bot/internal/client"
)

// Discord's limits on an embed. These are platform facts, not preferences, and a
// message that breaks one is refused rather than trimmed.
const (
	maxFieldValue = 1024
	maxFields     = 25
	maxEmbedTotal = 6000
)

// EventView is everything the event embed is drawn from. Board is nil until a comp has
// been locked, and that changes where the role fields come from: after a lock they are
// assignments, before it they are what each raider could play.
type EventView struct {
	Event   client.Event
	Signups []client.Signup
	Board   *client.Board
	// Icons are the application's class and spec emoji, empty when none are uploaded.
	// Passed in rather than read from a package variable so the builder stays pure.
	Icons IconSet
	// BannerURL is the guild's event image, shown under the roster. Empty for none.
	BannerURL string
}

// roleColumns is the order role fields appear in, with the label each gets. Tanks and
// healers first because that is the order a raid lead scans for gaps.
var roleColumns = []struct {
	Role  client.Role
	Label string
}{
	{client.RoleTank, "Tanks"},
	{client.RoleHealer, "Healers"},
	{client.RoleMelee, "Melee"},
	{client.RoleRanged, "Ranged"},
}

// roleGlyphs give each column a shape to scan for. Unicode rather than uploaded
// application emoji, so a self-hoster gets the same embed without setting anything up.
var roleGlyphs = map[client.Role]string{
	client.RoleTank:   "\U0001F6E1\uFE0F",
	client.RoleHealer: "\u2795",
	client.RoleMelee:  "\u2694\uFE0F",
	client.RoleRanged: "\U0001F3F9",
}

// difficultyColours tint the embed's left bar. Warcraft's own progression order, so a
// raid lead recognises the colour before reading the word.
var difficultyColours = map[client.Difficulty]int{
	client.DifficultyNormal: 0x4A9E4A,
	client.DifficultyHeroic: 0x3E7BC8,
	client.DifficultyMythic: 0x9B4DCA,
}

// defaultColour is for a Mythic+ group or a raid with no difficulty set.
const defaultColour = 0x5865F2

// roleShortNames are what a flex marker reads as: "Danthrax [tank/melee]".
var roleShortNames = map[client.Role]string{
	client.RoleTank:   "tank",
	client.RoleHealer: "healer",
	client.RoleMelee:  "melee",
	client.RoleRanged: "ranged",
}

// BuildEventEmbed renders the event message. Pure: everything it needs is in the view,
// so the interesting cases are testable without a Discord session or a service.
func BuildEventEmbed(v EventView) *discordgo.MessageEmbed {
	embed := &discordgo.MessageEmbed{
		Title:       v.Event.Title,
		Description: eventDescription(v.Event, signupTally(v.Signups)),
		Color:       embedColour(v.Event),
		Fields:      eventFields(v),
		Footer:      &discordgo.MessageEmbedFooter{Text: footerText(v)},
	}

	if v.BannerURL != "" {
		embed.Image = &discordgo.MessageEmbedImage{URL: v.BannerURL}
	}

	trimToTotalLimit(embed)
	return embed
}

// descriptionDivider sets the tally apart from the dates above and the role fields
// below, since it is the number a raid lead checks first.
const descriptionDivider = "───────────────────"

// eventDescription carries the times. They are Discord timestamps rather than
// formatted strings so that every member reads them in their own zone, which is the
// whole reason a guild spanning four countries can use one message.
func eventDescription(event client.Event, tally string) string {
	var b strings.Builder

	fmt.Fprintf(&b, "**Starts** %s (%s)", discordTime(event.StartsAt.Unix(), "F"), discordTime(event.StartsAt.Unix(), "R"))
	fmt.Fprintf(&b, "\n**Signups close** %s (%s)",
		discordTime(event.SignupDeadline.Unix(), "F"), discordTime(event.SignupDeadline.Unix(), "R"))

	// The type is only worth a line when it is not the default. A raid says "raid" in
	// its title; a Mythic+ group needs telling apart from one.
	if event.Type == client.EventMythicPlus {
		b.WriteString("\n**Mythic+**")
	}
	if event.Difficulty != nil {
		fmt.Fprintf(&b, "\n**Difficulty** %s", difficultyName(*event.Difficulty))
	}

	fmt.Fprintf(&b, "\n%s\n**%s**\n%s", descriptionDivider, tally, descriptionDivider)

	return b.String()
}

// difficultyName is title case rather than the enum's shouting: this is product copy a
// raider reads, not a constant.
func difficultyName(difficulty client.Difficulty) string {
	switch difficulty {
	case client.DifficultyNormal:
		return "Normal"
	case client.DifficultyHeroic:
		return "Heroic"
	case client.DifficultyMythic:
		return "Mythic"
	default:
		return string(difficulty)
	}
}

func embedColour(event client.Event) int {
	if event.Difficulty == nil {
		return defaultColour
	}
	if colour, ok := difficultyColours[*event.Difficulty]; ok {
		return colour
	}
	return defaultColour
}

func discordTime(unix int64, style string) string {
	return "<t:" + strconv.FormatInt(unix, 10) + ":" + style + ">"
}

func eventFields(v EventView) []*discordgo.MessageEmbedField {
	fields := make([]*discordgo.MessageEmbedField, 0, maxFields)

	if v.Board != nil {
		fields = append(fields, padToRow(lockedRoleFields(v))...)
	} else {
		fields = append(fields, padToRow(unlockedRoleFields(v))...)
	}

	fields = append(fields, padToRow(statusFields(v.Signups))...)

	if v.Board != nil && len(v.Board.Advisories) > 0 {
		fields = append(fields, advisoryField(v.Board.Advisories))
	}

	if len(fields) > maxFields {
		fields = fields[:maxFields]
	}
	return fields
}

// lockedRoleFields reads the comp. Bench lives here too: it is a property of the
// board, decided fresh by every lock, and never a signup status.
func lockedRoleFields(v EventView) []*discordgo.MessageEmbedField {
	seated := map[client.Role][]client.Assignment{}
	var bench []client.Assignment
	for _, slot := range v.Board.Slots {
		if slot.IsBench {
			bench = append(bench, slot)
			continue
		}
		seated[slot.Role] = append(seated[slot.Role], slot)
	}

	template, _ := v.Event.Template()
	fields := make([]*discordgo.MessageEmbedField, 0, len(roleColumns)+1)
	for _, column := range roleColumns {
		assignments := seated[column.Role]
		// An empty role the comp asks for is the most important thing on the message:
		// the gaps are what a raid lead is scanning for, so a missing tank shows as
		// 0/2 rather than as no field at all.
		if len(assignments) == 0 && needed(template, column.Role) == nil {
			continue
		}
		sort.SliceStable(assignments, func(i, j int) bool {
			return assignments[i].SlotIndex < assignments[j].SlotIndex
		})

		names := make([]string, len(assignments))
		for i, a := range assignments {
			names[i] = raiderName(v.Icons, a.Character, a.Role)
		}
		fields = append(fields, &discordgo.MessageEmbedField{
			Name:   roleHeading(column.Role, column.Label, len(assignments), needed(template, column.Role)),
			Value:  joinNames(names),
			Inline: true,
		})
	}

	if len(bench) > 0 {
		names := make([]string, len(bench))
		for i, a := range bench {
			names[i] = raiderName(v.Icons, a.Character, a.Role)
		}
		fields = append(fields, &discordgo.MessageEmbedField{
			Name:   fmt.Sprintf("\U0001FA91 Bench (%d)", len(bench)),
			Value:  joinNames(names),
			Inline: true,
		})
	}

	return fields
}

// unlockedRoleFields lists everyone who said they are coming, before any comp exists.
//
// Each raider appears once, under the role they ranked first, with a marker for what
// else they can play. Showing them under every playable role instead would double-count
// a flex raider and make "3/4 healers" a number a raid lead cannot act on.
func unlockedRoleFields(v EventView) []*discordgo.MessageEmbedField {
	coming := map[client.Role][]string{}
	var unassigned []string

	for _, s := range v.Signups {
		if s.Status != client.StatusConfirmed && s.Status != client.StatusLate {
			continue
		}
		if s.Character == nil {
			continue
		}

		// A raider who confirmed but set no role menu must still be visible. Dropping
		// them here once made the roster silently disagree with the count in the
		// footer, which is worse than an untidy field.
		roles := sortedByPriority(s.Character.Roles)
		if len(roles) == 0 {
			unassigned = append(unassigned, s.Character.Name+altMarker(s.Character.IsMain))
			continue
		}

		best := roles[0].Role
		coming[best] = append(coming[best], raiderName(v.Icons, s.Character, best))
	}

	template, _ := v.Event.Template()
	fields := make([]*discordgo.MessageEmbedField, 0, len(roleColumns)+1)
	for _, column := range roleColumns {
		names := coming[column.Role]
		if len(names) == 0 && needed(template, column.Role) == nil {
			continue
		}
		fields = append(fields, &discordgo.MessageEmbedField{
			Name:   roleHeading(column.Role, column.Label, len(names), needed(template, column.Role)),
			Value:  joinNames(names),
			Inline: true,
		})
	}

	if len(unassigned) > 0 {
		fields = append(fields, &discordgo.MessageEmbedField{
			Name:   fmt.Sprintf("\u26A0\uFE0F No role set (%d)", len(unassigned)),
			Value:  joinPlain(unassigned) + "\nThey need `/character roles` before the assigner can place them.",
			Inline: true,
		})
	}

	return fields
}

// fieldsPerRow is how many inline fields Discord puts on one line.
const fieldsPerRow = 3

// padToRow completes a section with invisible fields so the next section starts on a
// fresh line. Four role columns otherwise wrap as three then one, and the stray fourth
// sits alone under the first column looking like it belongs to it.
//
// A zero-width space is the standard trick here: Discord rejects an empty field name or
// value, but renders one containing only that as blank.
func padToRow(fields []*discordgo.MessageEmbedField) []*discordgo.MessageEmbedField {
	remainder := len(fields) % fieldsPerRow
	if len(fields) == 0 || remainder == 0 {
		return fields
	}

	const blank = "\u200b"
	for range fieldsPerRow - remainder {
		fields = append(fields, &discordgo.MessageEmbedField{Name: blank, Value: blank, Inline: true})
	}
	return fields
}

// statusFields are what raiders said about themselves, as opposed to what the comp did
// with them. Empty categories are left out: a raid with no tentatives should not carry
// a "Tentative: 0" field.
func statusFields(signups []client.Signup) []*discordgo.MessageEmbedField {
	late := map[client.SignupStatus][]string{}
	for _, s := range signups {
		if s.Character == nil {
			continue
		}
		switch s.Status {
		case client.StatusLate, client.StatusTentative, client.StatusDeclined, client.StatusAbsent:
			late[s.Status] = append(late[s.Status], lateName(s))
		case client.StatusConfirmed, client.StatusNoShow:
		}
	}

	var fields []*discordgo.MessageEmbedField
	for _, entry := range []struct {
		Status client.SignupStatus
		Label  string
	}{
		{client.StatusLate, "Late"},
		{client.StatusTentative, "Tentative"},
		{client.StatusDeclined, "Declined"},
		// Last because it is the least urgent to read: a planned absence is settled,
		// where a tentative is still a question the raid lead has to chase.
		{client.StatusAbsent, "Absent"},
	} {
		names := late[entry.Status]
		if len(names) == 0 {
			continue
		}
		fields = append(fields, &discordgo.MessageEmbedField{
			Name:   fmt.Sprintf("%s (%d)", entry.Label, len(names)),
			Value:  joinPlain(names),
			Inline: true,
		})
	}
	return fields
}

// advisoryField shows what the assigner wants a raid lead to know before pulling.
// These are not errors and are not hidden: "HEALER: 3, suggestion for 20 raiders is 4"
// is exactly the thing worth reading.
func advisoryField(advisories []client.Advisory) *discordgo.MessageEmbedField {
	lines := make([]string, len(advisories))
	for i, a := range advisories {
		if a.Role == "" {
			lines[i] = a.Message
			continue
		}
		lines[i] = string(a.Role) + ": " + a.Message
	}
	return &discordgo.MessageEmbedField{
		Name:  "Worth knowing",
		Value: truncate(strings.Join(lines, "\n"), "\n"),
	}
}

// signupTally counts signups by status. Confirmed and late share a bucket: a late
// arrival is still coming, just not on time.
func signupTally(signups []client.Signup) string {
	var confirmed, tentative, declined, absent int
	for _, s := range signups {
		switch s.Status {
		case client.StatusConfirmed, client.StatusLate:
			confirmed++
		case client.StatusTentative:
			tentative++
		case client.StatusDeclined:
			declined++
		case client.StatusAbsent:
			absent++
		case client.StatusNoShow:
		}
	}
	tally := fmt.Sprintf("%d in", confirmed)
	if tentative > 0 {
		tally += fmt.Sprintf(", %d tentative", tentative)
	}
	if declined > 0 {
		tally += fmt.Sprintf(", %d out", declined)
	}
	// Counted apart from "out" on purpose: four declines on one raid is a scheduling
	// problem, four absences is half the roster on holiday.
	if absent > 0 {
		tally += fmt.Sprintf(", %d absent", absent)
	}
	return tally
}

// footerText carries only the event id: the comp commands take it as an argument and
// there is nowhere else to read it off. The tally moved to the description, since that
// is the number a raid lead checks first.
func footerText(v EventView) string {
	return "event " + v.Event.ID
}

// raiderName renders one line of a roster field. A raider who registered more than one
// role carries a marker showing the others, which is the visible payoff of the whole
// multi-role model and should survive any tidying.
//
// Names are plain text, never mentions: a mention would ping twenty-five people every
// time the embed is edited, and it would break the grid.
func raiderName(icons IconSet, character *client.CharacterSummary, playing client.Role) string {
	if character == nil {
		// The service returns no summary when a character was deleted between the
		// signup read and the roster read. Saying so beats inventing a name.
		return "unknown raider"
	}
	name := icons.Markup(character.Class, character.Spec) + raiderioLink(character) + itemLevel(character) + altMarker(character.IsMain)
	if marker := flexMarker(character.Roles, playing); marker != "" {
		return name + " " + marker
	}
	return name
}

// raiderioLink makes a roster name clickable. Only the name is wrapped, so the item
// level and the markers after it stay outside the link and what a raider clicks is the
// word that names them.
//
// A masked link renders inside an embed, unlike in ordinary message content, but it is
// not free: the whole URL counts against the 1024-character field cap, around fifty
// characters per raider, so a deep column truncates sooner than it did before.
//
// The URL is absent against a service older than 0.2.0, and a plain name is the answer
// rather than one rebuilt here. Hard rule 5 says read the contract instead of assuming
// it, and the summary carries no region to rebuild the URL from anyway.
func raiderioLink(character *client.CharacterSummary) string {
	if character.RaiderIOURL == "" {
		return character.Name
	}
	return "[" + character.Name + "](" + character.RaiderIOURL + ")"
}

// altMarker says the raider is not on their main. Bracketed rather than parenthesised so
// it does not read as another role marker sitting next to "(offspec melee)", and worth
// showing because a raid lead reading a roster wants to know whose gear and logs the name
// they recognise does not apply to.
func altMarker(isMain bool) string {
	if isMain {
		return ""
	}
	return " [Alt]"
}

// flexMarker says what else a raider brings, and which way round it is.
//
// Listed under the role they ranked first, the others are off-spec. Listed under
// anything else, which is what a comp lock does when it moves someone, the salient fact
// is the opposite: they are playing off-spec and their main is elsewhere. A raid lead
// scanning a locked board needs to see that, and an unlabelled role name says neither.
func flexMarker(roles []client.RoleChoice, playing client.Role) string {
	if len(roles) < 2 {
		return ""
	}
	ordered := sortedByPriority(roles)

	if ordered[0].Role != playing {
		return "(main " + roleShortNames[ordered[0].Role] + ")"
	}

	others := make([]string, 0, len(ordered)-1)
	for _, rc := range ordered[1:] {
		others = append(others, roleShortNames[rc.Role])
	}
	if len(others) == 0 {
		return ""
	}
	return "(offspec " + strings.Join(others, "/") + ")"
}

// itemLevel is the one number a raid lead reads off a roster. Absent until the worker's
// Raider.IO sync has run, and shown as nothing rather than as a zero, because a fresh
// registration having no gear yet is not the same as being naked.
func itemLevel(character *client.CharacterSummary) string {
	if character.Ilvl == nil || *character.Ilvl <= 0 {
		return ""
	}
	return fmt.Sprintf(" `%.0f`", *character.Ilvl)
}

// lateName appends when a raider expects to arrive, as a Discord timestamp so it reads
// in their reader's zone. "I will be twenty minutes late" is only useful if the raid
// lead can act on it.
func lateName(s client.Signup) string {
	name := s.Character.Name + altMarker(s.Character.IsMain)
	if s.Status == client.StatusLate && s.LateUntil != nil {
		return name + " (from " + discordTime(s.LateUntil.Unix(), "t") + ")"
	}
	return name
}

// roleHeading is what a raid lead actually scans. The gap is the information, so the
// count reads as filled against wanted, with a marker once the role is covered.
func roleHeading(role client.Role, label string, filled int, want *int) string {
	glyph := roleGlyphs[role]
	if want == nil {
		return fmt.Sprintf("%s %s (%d)", glyph, label, filled)
	}
	if filled >= *want {
		return fmt.Sprintf("%s %s (%d/%d)", glyph, label, filled, *want)
	}
	return fmt.Sprintf("%s %s (%d/%d)", glyph, label, filled, *want)
}

// needed reads a role's target off the comp template. Melee and ranged are caps rather
// than requirements, but they are still the number a raid lead is filling towards.
func needed(template client.CompTemplate, role client.Role) *int {
	switch role {
	case client.RoleTank:
		return template.Tanks
	case client.RoleHealer:
		return template.Healers
	case client.RoleMelee:
		return template.MaxMelee
	case client.RoleRanged:
		return template.MaxRanged
	default:
		return nil
	}
}

// sortedByPriority orders a role menu the way the raider ranked it: 1 is their main
// spec, 3 is if the raid is desperate.
func sortedByPriority(roles []client.RoleChoice) []client.RoleChoice {
	ordered := make([]client.RoleChoice, len(roles))
	copy(ordered, roles)
	sort.SliceStable(ordered, func(i, j int) bool {
		return ordered[i].Priority < ordered[j].Priority
	})
	return ordered
}

// joinNames stacks a roster one raider per line and numbers it. Numbering is what makes
// "we have eleven" answerable at a glance, and one per line is what turns an inline
// field into the column a raid lead scans.
func joinNames(names []string) string {
	if len(names) == 0 {
		return "\u2014"
	}
	numbered := make([]string, len(names))
	for i, name := range names {
		numbered[i] = fmt.Sprintf("`%d.` %s", i+1, name)
	}
	return truncate(strings.Join(numbered, "\n"), "\n")
}

// joinPlain is for the lists where position carries no meaning, such as who declined.
func joinPlain(names []string) string {
	return truncate(strings.Join(names, ", "), ", ")
}

// truncate keeps a field inside Discord's per-value cap. A twenty-person list with
// flex markers gets close, and an embed that exceeds it is refused outright, so the
// tail becomes a count instead.
func truncate(value, separator string) string {
	if len(value) <= maxFieldValue {
		return value
	}

	parts := strings.Split(value, separator)
	kept := make([]string, 0, len(parts))
	length := 0
	for _, part := range parts {
		// Reserve room for the longest possible tail, so dropping one more entry can
		// never be what pushes the field over.
		const tailBudget = 24
		next := length + len(part) + len(separator)
		if next+tailBudget > maxFieldValue {
			break
		}
		kept = append(kept, part)
		length = next
	}

	return strings.Join(kept, separator) + separator + fmt.Sprintf("and %d more", len(parts)-len(kept))
}

// trimToTotalLimit drops fields from the end until the whole embed fits. Discord
// refuses a message over the total, so losing the last field beats losing all of them.
func trimToTotalLimit(embed *discordgo.MessageEmbed) {
	for embedLength(embed) > maxEmbedTotal && len(embed.Fields) > 0 {
		embed.Fields = embed.Fields[:len(embed.Fields)-1]
	}
}

func embedLength(embed *discordgo.MessageEmbed) int {
	total := len(embed.Title) + len(embed.Description)
	if embed.Footer != nil {
		total += len(embed.Footer.Text)
	}
	for _, f := range embed.Fields {
		total += len(f.Name) + len(f.Value)
	}
	return total
}
