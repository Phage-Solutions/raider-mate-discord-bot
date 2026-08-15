# raider-mate-bot: specification

Discord bot for Raider Mate. This document specifies behaviour, not implementation.

The full domain spec (schema, assignment algorithm, tier rationale) lives in
`raider-mate-service/docs/design.md`. This file covers the Discord surface: commands,
embeds, interaction flows, and the API contract they imply.

Status: pre-implementation. Nothing here is built yet.

**Vocabulary:** the privileged user is a **raid lead**, not an officer. The service
uses that term; match it in code, copy, and docs.

---

## 1. Scope

**This bot does:** render Discord UI, collect user input, call the service API, and
display what comes back.

**This bot does not:** decide whether a signup is valid, rank candidates for a role,
compute a comp, decide who is benched, validate raid buffs, or determine tier access.
Those are service-side. If a feature below seems to require the bot to make such a
decision, the decision belongs behind an API call.

**Non-goals for v0.1:** slash-command autocomplete over character names, recurring
event creation from Discord (dashboard only), any agentic or natural-language input.

---

## 2. Command surface

Slash commands. None require message content intent. See "Command registration"
below for scope, which differs between development and production.

| Command | Who | Purpose |
|---|---|---|
| `/raid create` | Raid lead | Create a raid event. Opens a modal. |
| `/mplus create` | Any | Create a Mythic+ group. Opens a modal. |
| `/event edit <id>` | Raid lead | Edit title, time, or deadline. |
| `/event cancel <id>` | Raid lead | Cancel, notify signed-up raiders. |
| `/comp lock <id>` | Raid lead | Run the assigner on an AUTO comp. |
| `/comp show <id>` | Any | Show current comp without modifying. |
| `/character add` | Any | Register a character. Opens a modal. |
| `/character roles` | Any | Edit which roles a character can play. |
| `/character remove` | Any | Unregister a character. |
| `/absence` | Any | Declare an absence window. |
| `/roster` | Any | Summary of guild roster, links to dashboard. |
| `/audit` | Any | Current iLvl and score summary. Premium adds detail. |
| `/help` | Any | Command reference and dashboard link. |

**Raid lead** means a configurable Discord role, set once per guild. The bot reads the
member's roles from the interaction payload and sends them to the service; the service
decides whether the action is permitted. The bot never decides this locally.

### Command naming

Subcommand groups (`/event`, `/comp`, `/character`) rather than flat commands, to keep
the command list readable and leave room for growth without polluting the slash menu.

### Command registration

Commands can only be registered via HTTP endpoint. There is no portal UI for this.

Scope is decided by which endpoint you POST to:

| Scope | Endpoint |
|---|---|
| Global | `/applications/{app_id}/commands` |
| Guild | `/applications/{app_id}/guilds/{guild_id}/commands` |

**Use guild scope during development.** Guild commands update instantly, which is
Discord's own recommendation for quick testing. Make the dev guild ID a config value
so production can register globally without a code change.

Global commands have read-repair. If a command is updated and a user invokes it before
their client has the new version, Discord does an internal version check, rejects the
invocation, and triggers a reload for that command. Stale global commands therefore
self-heal rather than silently misbehaving. (Earlier drafts of this document claimed a
propagation delay of up to an hour. That figure is common in third-party guides but is
not in the current official documentation, which describes read-repair instead.)

**Rate limit: 200 application command creates per day, per guild.** Do not wire
registration into every bot start during development. Register on demand, or only when
the command definitions have actually changed.

### Installation contexts are a separate axis

Installation context is where the app can be installed: to a guild, to a user account,
or both. It is configured at the application level in the developer portal, and
per-command via `integration_types`.

**Raider Mate is Guild Install only.** It is inherently a per-guild tool; a
user-installed copy has no roster to act on.

Note that `integration_types` and `contexts` can only be set on globally-scoped
commands. Guild-scoped commands do not accept those fields, so a dev guild command and
its production global counterpart are not quite the same object. Worth remembering if
something behaves differently after the switch to global.

---

## 3. The event embed

The embed is the product's face. Twenty-five people look at it every raid night.

### Anatomy

```
[Title]            Heroic Nerub-ar Palace
[Description]      Tuesday 20:00 CET, signups close Mon 12:00
                   <t:1234567890:R>

[Field: Tanks]     2/2   OK
                   Danthrax, Grimwall

[Field: Healers]   3/4
                   Lightbringer, Mossheart, Sunwell

[Field: Melee]     5/7
                   ...

[Field: Ranged]    6/7
                   ...

[Field: Bench]     3
                   ...

[Field: Late]      1
                   Thornrend (from 20:30)

[Field: Tentative] 2
[Field: Declined]  4

[Footer]           18 confirmed, updated 19:42
[Buttons]          [Sign up] [Tentative] [Decline] [Withdraw]
```

### Where each field comes from

This matters more than it looks, because two different sources feed one embed:

- **Tanks, Healers, Melee, Ranged, Bench** come from the **comp**, not from signup
  statuses. Bench membership lives on `comp_slots.is_bench` and is decided fresh by
  every lock.
- **Late, Tentative, Declined** come from **signup status**, which is what the raider
  self-reported.

Before a comp exists, the role fields show signups grouped by what each character
*can* play, with a note that nothing is assigned yet. After a lock, they show
assignments.

### Display rules

- **Times use Discord timestamps** (`<t:unix:F>` for absolute, `<t:unix:R>` for
  relative) so every member sees their own timezone. Never render a formatted time
  string bot-side.
- **Flex players carry a role marker** next to their name, showing which other roles
  they registered: `Danthrax [tank/melee]`. This is the visible payoff of the
  multi-role model and should not be dropped for tidiness.
- **Counts show filled against the comp's needs** (`3/4`), with a marker when a role
  is satisfied. Raid leads scan for the gaps, so the gaps must be the visually obvious
  thing.
- **Empty categories are omitted**, not shown as empty fields. A raid with no
  tentatives should not have a "Tentative: 0" field taking up space.
- **Names are display names**, not mentions, in the roster fields. Mentions would ping
  and would break the visual grid.
- **Advisories from the assigner are shown**, briefly, beneath the roster. "HEALER: 3,
  suggestion for 20 raiders is 4" is exactly what a raid lead needs to see before
  pulling. Do not hide them, and do not treat them as errors.

### Discord limits that constrain this

These are platform limits, not preferences, and the design has to live inside them:

- **25 fields maximum per embed.** Fine for role categories, but rules out one field
  per raider on a large roster.
- **1024 characters per field value.** A 20-person DPS list with role markers can
  approach this. Truncate with "and 4 more" and link to the dashboard rather than
  letting the embed fail to render.
- **6000 characters total across the embed.** Same concern, checked before send.
- **5 buttons per action row, 5 rows per message.** The four signup buttons fit one
  row comfortably, leaving room for raid lead controls on a second row.
- **Select menus allow 25 options.** Relevant for character selection when a user has
  many alts, not for role selection.

If a roster genuinely exceeds what an embed can show, the embed shows counts and a
dashboard link rather than a truncated mess.

---

## 4. Interaction flows

### 4.1 Signup, returning user

1. User clicks `Sign up`.
2. Bot defers immediately (ephemeral).
3. Bot fetches the user's characters and registered roles from the service.
4. **One character:** ephemeral string select of that character's roles, multi-select,
   ordered by priority.
   **Multiple characters:** character select first, then role select.
5. User confirms. Bot POSTs the signup.
6. Bot edits the original event message with the updated embed.
7. Ephemeral confirmation to the user, auto-dismissed.

Target: two clicks for the common case.

### 4.2 Signup, first-time user

Same entry point, but the service returns no characters.

1. Bot opens a modal: character name, realm.
2. Bot POSTs to the service, which performs the Raider.IO lookup and returns class,
   spec, iLvl, and score.
3. Bot shows the role select, defaulted to the roles plausible for that class and spec
   (the service supplies these; the bot does not hardcode class-to-role mappings).
4. User confirms roles, then the signup proceeds as in 4.1.

Roughly twenty seconds, once. Everything after is two clicks.

**Modals cannot be opened from a deferred interaction.** The first-time path must
open the modal as the immediate response, which means the bot needs to know whether
the user has characters *before* deferring. Either cache that per user, or accept a
round trip and open the modal from a fresh interaction. This constraint is easy to
miss and shapes the implementation.

### 4.3 Withdraw

`Withdraw` appears only for users already signed up. Single click, no confirmation
dialog, since the action is trivially reversible by signing up again. Embed updates.

If a locked comp already assigned the raider, withdrawing notifies the raid lead
rather than silently reshuffling. The bot does not re-run the assigner.

### 4.4 Tentative and Decline

Single click each. Decline asks for no reason; an optional note can come later from
the dashboard. Both update the embed immediately.

### 4.5 Late

Not a button. `Late` is set by picking `Tentative` and then supplying a time, or via a
follow-up ephemeral prompt after signup. The distinction matters: `LATE` carries a
`late_until` timestamp, so it needs input that a single button cannot provide.

### 4.6 Comp lock

A comp is keyed `(event_id, name)` and carries a `mode`:

| Mode | Owner | Behaviour |
|---|---|---|
| `AUTO` | The assigner | `lock` recomputes every slot from current signups |
| `MANUAL` | The raid lead | The assigner never runs; the board is whatever was saved |

Flow for an AUTO comp:

1. Raid lead runs `/comp lock <id>`.
2. Bot calls the lock endpoint. Service returns the comp with a reason string per
   assignment and any advisories.
3. Bot posts the result as an ephemeral message to the raid lead, reasons visible.
4. Raid lead confirms. Bot updates the public embed and DMs each assigned raider their
   role.

**`lock` on a MANUAL comp returns `ErrCompIsManual` and writes nothing.** The bot must
surface this as a plain sentence, not an error dump: something like "That comp is
hand-built, so the assigner will not touch it. Edit it in the dashboard, or convert it
to auto first." A raid lead who hand-built a board and then hit lock expects their work
to survive, and it does.

**Manual comps are edited in the dashboard, not in Discord.** Manual saves are
whole-board writes; the editing surface holds the entire board and submits it at once.
Rebuilding that interaction out of Discord select menus is not worth it, and partial
per-slot edits from two raid leads would need conflict resolution that the whole-board
model exists to avoid.

**Reasons are shown, not hidden.** "Why did Bob get benched" is the most common raid
lead question and the answer already exists in the API response.

**Nothing validates a manual board.** A healer placed as a tank, or an eleven-man
Mythic roster, is written exactly as asked. The bot renders it without comment. The
raid lead is the authority.

### 4.7 Reminders

The service polls `scheduled_jobs` on a ticker and decides when a reminder is due.
How that reaches the bot is still open; see section 10.

| Trigger | Recipients | Content |
|---|---|---|
| 24h before | Undecided only | Nudge to sign up. Never ping people who already responded. |
| 1h before | Confirmed roster | Their assigned role, invite reminder. |
| Signup deadline | Raid lead | Signups locked, comp needs finalising. |
| 2h before, comp unlocked | Raid lead | Nag. |

All reminders are DMs. **DMs can fail** if the user has them closed; the bot must
handle this without erroring the whole job, and should tell the raid lead which raiders
could not be reached.

---

## 5. Component custom_id scheme

Discord's `custom_id` is capped at **100 characters** and is the only state carried
back on a component interaction. The scheme must encode enough to route without a
lookup, and must not encode anything sensitive.

Proposed shape:

```
rm:<version>:<action>:<entityId>
rm:1:signup:0192f3c8-...
rm:1:role-select:0192f3c8-...
rm:1:withdraw:0192f3c8-...
```

- **Prefix `rm:`** so the bot can cleanly ignore components from other bots.
- **Version segment** so old messages with stale component IDs can be detected and
  answered with "this event message is out of date" rather than misrouted. Pinned
  messages live a long time; this will matter.
- A UUID is 36 characters, leaving comfortable headroom inside 100.
- **Never encode user IDs or role names.** The interaction payload already carries the
  invoking user; trusting a client-supplied ID in a custom_id would be an
  impersonation vector.

---

## 6. Errors and failure modes

The bot sits between two systems that both fail in normal operation.

| Failure | Behaviour |
|---|---|
| Service unreachable | Ephemeral: "Can't reach the roster service, try again in a minute." Never leave the interaction hanging. |
| Service returns 4xx | Show the service's message if it is user-facing, otherwise a generic failure. Do not leak internal errors into Discord. |
| `ErrCompIsManual` | Explain in one sentence and point at the dashboard. Not an error dump. |
| Interaction expired (>3s undeferred) | Should be impossible if the defer rule is followed. Log loudly if seen. |
| Message deleted by a user | Service still holds the event. `/comp show` should offer to repost. |
| DM blocked | Continue the job. Report unreachable users to the raid lead. |
| Rate limited by Discord | Back off and retry. Embed edits are the likely hot spot on a busy signup. |

**Embed edits should be coalesced.** Twenty people signing up in the same minute
should not produce twenty sequential edits of the same message. Debounce, then edit
once.

---

## 7. Permissions

The bot requires: `applications.commands`, send messages, embed links, and read
message history in the channels it posts to. It does **not** require message content
intent, presence intent, or guild members intent.

Raid lead permission is checked service-side. The bot passes the invoking member's
role IDs along with the request and renders whatever controls the response's HATEOAS
links permit. If the API returns no `bench` link, no bench control is rendered,
regardless of what the bot believes about the user.

---

## 8. Edge cases worth deciding now

- **A user signs up on an alt after signing up on a main.** Allowed? Probably yes for
  M+, no for raids. Service decides; the bot surfaces the resulting error clearly.
- **A raid lead edits the event after signups exist.** Existing assignments may no
  longer fit. The embed should show the mismatch rather than silently reshuffling.
- **Two raid leads lock the same comp simultaneously.** Service resolves; the bot shows
  the loser a "comp was changed by someone else" message rather than a raw conflict
  error.
- **A raid lead locks an AUTO comp that a colleague just converted to MANUAL.** Same
  `ErrCompIsManual` path as 4.6. Say what happened; do not retry.
- **A character is renamed or transferred realms in game.** Raider.IO lookups will
  fail. The bot should offer to re-register rather than showing a stale error forever.
- **The event's guild loses Premium mid-cycle.** Premium-only embed content
  disappears from subsequent renders. Existing messages are not retroactively edited.

---

## 9. API contract implied by this spec

Endpoints the bot needs, in build order. This list is the bot's ask; the service repo
owns the actual shapes.

```
GET    /api/guilds/{guildId}/members/{discordUserId}/characters
POST   /api/guilds/{guildId}/characters
PUT    /api/characters/{id}/roles

POST   /api/guilds/{guildId}/events
GET    /api/events/{id}
PATCH  /api/events/{id}
DELETE /api/events/{id}

GET    /api/events/{id}/signups
POST   /api/events/{id}/signups
PATCH  /api/events/{id}/signups/{sid}
DELETE /api/events/{id}/signups/{sid}

GET    /api/events/{id}/comps
GET    /api/events/{id}/comps/{name}
POST   /api/events/{id}/comps/{name}/lock
```

Every response carries `_links`. The bot renders controls from those links only.

Manual comp saving and AUTO/MANUAL conversion are dashboard concerns and are
deliberately absent from this list.

---

## 10. Open questions

1. **Does the service push reminders to the bot, or does the bot poll?** The service
   already runs a ticker over `scheduled_jobs`, so it knows when a reminder is due.
   Push (service calls a bot endpoint) is cleaner but couples deployment. Polling is
   simpler for self-hosters. Leaning polling for v0.1.
2. **Where does the guild's raid lead role get configured?** The dashboard is the
   obvious home, but that forces every guild through the dashboard before the bot is
   usable. A `/config` command may be needed for v0.1 even though it feels like
   dashboard work.
3. **One bot instance per guild, or shared?** Shared for the hosted instance,
   single-guild for self-hosters. The code should not assume either.
4. **Does `/comp lock` need a comp name argument?** A comp is keyed
   `(event_id, name)`, so an event can hold several. `/comp lock <id>` with no name is
   ambiguous once a second comp exists. Either default to a well-known name or add the
   argument.

**Resolved since the last draft:** character identity across guilds. `users` is unique
on `(discord_id, discord_guild_id)`, so the same person in two guilds is two user rows
with their own characters. The bot never needs to reconcile them.

---

## 11. v0.1 scope

Everything above is the target. v0.1 is narrower:

- `/raid create`, `/character add`, `/character roles`
- Signup, Tentative, Decline, Withdraw buttons
- Multi-role select
- Event embed with role fields and counts
- `/comp lock` on AUTO comps, with advisories surfaced
- 24h and 1h reminders

Deferred: `/mplus create`, `/absence`, `/audit`, waitlist promotion, Late handling,
alt selection when a user has multiple characters (v0.1 assumes one), and any manual
comp editing from Discord.
