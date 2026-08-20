package client

import (
	"encoding/json"
	"time"
)

// Role is what a character can play. The service's role_enum splits DPS by range,
// because a comp template counts melee and ranged separately.
type Role string

const (
	RoleTank   Role = "TANK"
	RoleHealer Role = "HEALER"
	RoleMelee  Role = "MDPS"
	RoleRanged Role = "RDPS"
)

// SignupStatus is what a raider said about an event. Bench is deliberately absent: it
// lives on the comp, not the signup, and the service dropped it from the enum.
//
// ABSENT and DECLINED are both a no, and the difference is scope: DECLINED answers this
// event, ABSENT says the raider is out for a stretch. Both are the raider's to set.
// NO_SHOW is the raid lead's alone, so the bot never sends it.
type SignupStatus string

const (
	StatusConfirmed SignupStatus = "CONFIRMED"
	StatusTentative SignupStatus = "TENTATIVE"
	StatusDeclined  SignupStatus = "DECLINED"
	StatusLate      SignupStatus = "LATE"
	StatusAbsent    SignupStatus = "ABSENT"
	StatusNoShow    SignupStatus = "NO_SHOW"
)

// EventType distinguishes a raid night from a Mythic+ group. The assigner is the same
// function either way, with a different template and a different ranking key.
type EventType string

const (
	EventRaid       EventType = "RAID"
	EventMythicPlus EventType = "MYTHIC_PLUS"
)

// Difficulty is null for Mythic+.
type Difficulty string

const (
	DifficultyNormal Difficulty = "NORMAL"
	DifficultyHeroic Difficulty = "HEROIC"
	DifficultyMythic Difficulty = "MYTHIC"
)

// CompMode says who owns a comp's slots. An AUTO comp is the assigner's and is
// recomputed on every lock; a MANUAL one is the raid lead's and is never touched.
type CompMode string

const (
	ModeAuto   CompMode = "AUTO"
	ModeManual CompMode = "MANUAL"
)

// RequestState is where a late signup request stands.
type RequestState string

const (
	RequestPending  RequestState = "PENDING"
	RequestApproved RequestState = "APPROVED"
	RequestRejected RequestState = "REJECTED"
)

// RoleChoice is one role a character can play, with the priority the raider gave it:
// 1 is their main spec, 3 is if the raid is desperate.
type RoleChoice struct {
	Role     Role  `json:"role"`
	Priority int16 `json:"priority"`
}

// Character is a registered character. Synced reports whether the worker has filled in
// the Raider.IO fields yet; on a fresh registration everything below Region is nil.
type Character struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Realm string `json:"realm"`
	// RaiderIOURL is the character's page on Raider.IO. Empty against a service older
	// than 0.2.0, so read it as "link if you have one" rather than as always present.
	RaiderIOURL string   `json:"raiderio_url,omitempty"`
	Region      string   `json:"region"`
	Class       *string  `json:"class,omitempty"`
	Spec        *string  `json:"spec,omitempty"`
	Ilvl        *float64 `json:"ilvl,omitempty"`
	MplusScore  *float64 `json:"mplus_score,omitempty"`
	IsMain      bool     `json:"is_main"`
	Synced      bool     `json:"synced"`
	Links       Links    `json:"_links"`
}

// CharacterSummary is a character as it appears inside a signup row or a comp slot:
// enough to draw the line, including the role menu the flex marker is built from.
type CharacterSummary struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Realm string `json:"realm"`
	// RaiderIOURL is what makes a roster name clickable. The summary carries no region,
	// so this is the only way to reach the character's page without a second request.
	RaiderIOURL string       `json:"raiderio_url,omitempty"`
	Class       *string      `json:"class,omitempty"`
	Spec        *string      `json:"spec,omitempty"`
	Ilvl        *float64     `json:"ilvl,omitempty"`
	Roles       []RoleChoice `json:"roles"`
	IsMain      bool         `json:"is_main"`
}

// CompTemplate is how many of each the raid wants. Every field is optional and the
// service derives what is missing, so a zero template is the default rather than an
// empty raid.
type CompTemplate struct {
	Tanks     *int `json:"tanks,omitempty"`
	Healers   *int `json:"healers,omitempty"`
	MaxMelee  *int `json:"max_melee,omitempty"`
	MaxRanged *int `json:"max_ranged,omitempty"`
}

// Event is a raid night or a Mythic+ group. MessageID and ChannelID are the bot's own
// post, written back after the embed goes up: without them the service has nowhere to
// send a deadline or comp-nag notification.
type Event struct {
	ID             string          `json:"id"`
	Type           EventType       `json:"type"`
	Title          string          `json:"title"`
	StartsAt       time.Time       `json:"starts_at"`
	SignupDeadline time.Time       `json:"signup_deadline"`
	CompTemplate   json.RawMessage `json:"comp_template"`
	MessageID      *string         `json:"message_id,omitempty"`
	ChannelID      *string         `json:"channel_id,omitempty"`
	Difficulty     *Difficulty     `json:"difficulty,omitempty"`
	// ReminderLeadMinutes is what the service resolved, not what was asked for: a create
	// that named none reads back the guild's default. 0 means no reminder.
	ReminderLeadMinutes *int  `json:"reminder_lead_minutes,omitempty"`
	Links               Links `json:"_links"`
}

// Template decodes the event's comp template. A template that will not parse is the
// service's problem, not something to render around.
func (e Event) Template() (CompTemplate, error) {
	var tmpl CompTemplate
	if len(e.CompTemplate) == 0 {
		return tmpl, nil
	}
	if err := json.Unmarshal(e.CompTemplate, &tmpl); err != nil {
		return CompTemplate{}, err
	}
	return tmpl, nil
}

// Signup is what one character said. AssignedRole stays nil until a comp is locked.
// Character is populated on list responses and nil on the single-signup writes, where
// the caller already knows which character it named.
type Signup struct {
	ID           string            `json:"id"`
	CharacterID  string            `json:"character_id"`
	Character    *CharacterSummary `json:"character,omitempty"`
	Status       SignupStatus      `json:"status"`
	AssignedRole *Role             `json:"assigned_role,omitempty"`
	LateUntil    *time.Time        `json:"late_until,omitempty"`
	Note         *string           `json:"note,omitempty"`
	// AllowedStatuses is what the calling actor may write on this signup, empty for a
	// caller who cannot act on it at all. The bot's signup buttons live on a message
	// every viewer sees the same way, so it has nothing to gate on this; it is read so
	// the contract is represented and a caller that does build a per-actor surface has
	// the field without another round of API work.
	AllowedStatuses []SignupStatus `json:"allowed_statuses,omitempty"`
	Links           Links          `json:"_links"`
}

// LateRequest is what a signup becomes once the deadline has passed. The player's
// write is not refused, it is queued for a raid lead.
type LateRequest struct {
	ID          string            `json:"id"`
	CharacterID string            `json:"character_id"`
	Character   *CharacterSummary `json:"character,omitempty"`
	Status      SignupStatus      `json:"status"`
	Note        *string           `json:"note,omitempty"`
	LateUntil   *time.Time        `json:"late_until,omitempty"`
	State       RequestState      `json:"state"`
	Links       Links             `json:"_links"`
}

// Assignment is one seat on a comp board. Reason is the assigner's sentence for why
// this raider got this seat, and it is meant to be shown: "why did Bob get benched" is
// the question a raid lead asks, and the answer already exists.
type Assignment struct {
	CharacterID string            `json:"character_id"`
	Character   *CharacterSummary `json:"character,omitempty"`
	Role        Role              `json:"role"`
	SlotIndex   int16             `json:"slot_index"`
	IsBench     bool              `json:"is_bench"`
	Reason      string            `json:"reason"`
}

// Advisory is the assigner reporting something a raid lead should see before pulling,
// such as three healers where twenty raiders usually want four. It is information, not
// an error, and the raid lead decides.
type Advisory struct {
	Role    Role   `json:"role,omitempty"`
	Message string `json:"message"`
}

// CompInfo is a comp's identity without its slots, as listed on an event.
type CompInfo struct {
	Name  string   `json:"name"`
	Mode  CompMode `json:"mode"`
	Links Links    `json:"_links"`
}

// Board is a comp with its slots filled in.
type Board struct {
	Name       string       `json:"name"`
	Mode       CompMode     `json:"mode"`
	Slots      []Assignment `json:"slots"`
	Advisories []Advisory   `json:"advisories,omitempty"`
	Links      Links        `json:"_links"`
}

// NotificationKind is why the service wants a message sent.
type NotificationKind string

const (
	Reminder24h NotificationKind = "REMINDER_24H"
	// ReminderPreEvent fires a configurable number of minutes before the start, 30 by
	// default. It reaches everyone signed up, seated or not.
	ReminderPreEvent NotificationKind = "REMINDER_PRE_EVENT"
	// Reminder1h is what ReminderPreEvent was called before the lead time became a
	// setting. Kept for one release so a bot deployed ahead of the service still
	// delivers what the old service queued; remove it once both are past 0.4.0.
	Reminder1h       NotificationKind = "REMINDER_1H"
	SignupDeadline   NotificationKind = "SIGNUP_DEADLINE"
	LateRequestFiled NotificationKind = "LATE_REQUEST_FILED"
	// RosterUpdated carries no message. A raider's cached gear or score moved, so the
	// event message is showing stale numbers and needs redrawing.
	RosterUpdated NotificationKind = "ROSTER_UPDATED"
	// CompSlotDropped is a raider leaving the assignment pool after a comp was locked.
	// The service takes their slot with the write; the raid lead is told rather than
	// left to notice a hole (design.md section 4.3).
	CompSlotDropped NotificationKind = "COMP_SLOT_DROPPED"
	// EventCreated is an event made somewhere the bot cannot post from, the dashboard
	// being the one that exists. It carries no message: the bot reads the event back,
	// posts its signup sheet in the guild's events channel, and tells the service where
	// it landed.
	EventCreated NotificationKind = "EVENT_CREATED"
)

// NotificationTarget is who receives it: one raider by DM, a channel post mentioning
// the guild's raid-lead roles or a named list of raiders, or nobody at all in the case
// of an edit to a message the bot already posted.
type NotificationTarget string

const (
	TargetUser NotificationTarget = "USER"
	TargetRole NotificationTarget = "ROLE"
	// TargetChannel is a channel post mentioning the users in DiscordIDs. Separate from
	// TargetRole because a role and a user render with different mention syntax.
	TargetChannel NotificationTarget = "CHANNEL"
	TargetMessage NotificationTarget = "MESSAGE"
)

// Notification is one queued message. Payload stays raw because its shape depends on
// Kind; the delivery code unmarshals it into the matching struct.
type Notification struct {
	ID             string             `json:"id"`
	DiscordGuildID string             `json:"discord_guild_id"`
	EventID        string             `json:"event_id"`
	Kind           NotificationKind   `json:"kind"`
	TargetKind     NotificationTarget `json:"target_kind"`
	DiscordID      *string            `json:"discord_id,omitempty"`
	RoleIDs        []string           `json:"role_ids,omitempty"`
	DiscordIDs     []string           `json:"discord_ids,omitempty"`
	ChannelID      *string            `json:"channel_id,omitempty"`
	Payload        json.RawMessage    `json:"payload"`
	Links          Links              `json:"_links"`
}

// RaidLeadRoles is the guild's mapping from Discord role to raid-lead capability.
type RaidLeadRoles struct {
	RoleIDs []string `json:"role_ids"`
	Links   Links    `json:"_links"`
}
