# raider-mate-bot: specification

Discord bot for Raider Mate. This document specifies behaviour, not implementation.

The full domain spec (schema, assignment algorithm, tier rationale) lives in
`raider-mate-service/docs/design.md`. This file covers the Discord surface: commands,
embeds, interaction flows, and the API contract they imply.

Status: v0.1 in progress. Section 11 says what is built and what is deferred.

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
| `/raid create` | Raid lead | Create a raid event. Command options, not a modal. |
| `/dungeon create` | Raid lead | Create a Mythic+ group. See the note below on who. |
| `/event edit <id>` | Raid lead | Edit title, time, or deadline. |
| `/event cancel <id>` | Raid lead | Cancel, notify signed-up raiders. |
| `/comp lock <id>` | Raid lead | Run the assigner on an AUTO comp. |
| `/comp show <id>` | Any | Show current comp without modifying. |
| `/character add` | Any | Register a character. Opens a modal. |
| `/character roles` | Any | Edit which roles a character can play. |
| `/character main` | Any | Move the main flag to another of your characters. |
| `/character remove` | Any | Unregister a character. |
| `/absence` | Any | Declare an absence window. |
| `/roster` | Any | Summary of guild roster, links to dashboard. |
| `/audit` | Any | Current iLvl and score summary. Premium adds detail. |
| `/config channel` | Guild admin | Choose which channel event messages are posted in. |
| `/config timezone` | Guild admin | The IANA zone raid times are typed in. |
| `/config mentions` | Guild admin | Which roles a new event message pings. |
| `/config banner` | Guild admin | Image shown on every event card. |
| `/help` | Any | Command reference and dashboard link. |

**Raid lead** means a configurable Discord role, set once per guild. The bot reads the
member's roles from the interaction payload and sends them to the service; the service
decides whether the action is permitted. The bot never decides this locally.

### Raids and dungeons are separate commands

`/raid create` and `/dungeon create` post the same card and share one code path,
differing only in the event type they send. That type is what the service reads: a
Mythic+ group is sized at five with one tank and one healer by `ModeMythicPlus`, and
candidates are ranked on Mythic+ score rather than item level. None of that sizing lives
in the bot (hard rule 3), which is why `/dungeon create` offers no tank or healer counts
and sends an empty comp template.

A dungeon takes no difficulty either: `raid_difficulty` is null for Mythic+.

Earlier drafts called this `/mplus create`. `/dungeon` is what a guild says.

### Why event creation is command options and not a modal

A modal holds text inputs and nothing else worth having here. Discord has **no date
component at all**, in modals or on messages, so a date picker is not something this
bot declined to build. Select menus in modals do now exist, wrapped in a `Label`
component, but `discordgo` v0.29.0 has no `Label` and building one by hand means
hand-rolling raw component JSON.

Command options give one thing a modal cannot: `difficulty` renders as a real picker
with Normal, Heroic and Mythic. Everything else is typed either way, so the modal was
costing a form without buying a control.

Times are therefore parsed rather than picked, and parsing is deliberately forgiving:
`tomorrow 20:00`, `sat 20:00`, `20:00`, `2026-09-01 20:00`, `01.09.2026 20:00`, or any
of those with an explicit offset. A bare clock time means the next time it comes round.
Naming today's weekday means next week, since nobody types "saturday" on Saturday
afternoon to mean a raid that already started. `signups_close` defaults to a day before
the raid, or to the start time when the raid is sooner than that, because closing
signups before they opened helps nobody.

Resolving any of that needs a zone. Discord exposes none, for a guild or a user, so
`/config timezone` stores an IANA name in `guild_settings.timezone`. IANA rather than a
fixed offset: a raid schedule outlives a daylight saving change, and `+02:00` quietly
becomes wrong in October. A guild that has set no zone has times read as UTC, and the
error message on a failed parse says so.

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
[Description]      Starts <t:1788292800:F> (<t:1788292800:R>)
                   Signups close <t:1788177600:F> (<t:1788177600:R>)
                   Difficulty Heroic

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
[Field: Absent]    1

[Footer]           18 in, 2 tentative, 4 out, 1 absent | event 0192f3c8-...
[Buttons]          [Sign up] [Tentative] [Late] [Decline] [Absent]
                   [Withdraw]
```

Late and Absent are fields under the roster, not in it. They read as what raiders said
about themselves, after what the comp did with them.

The `:F` style renders the full date and time, so the date is not a separate line. The
event id is in the footer because `/comp show` and `/comp lock` take it as an argument
and there is nowhere else to read it from.

### Artwork and icons

Two decorations, both optional, both degrading to plain text rather than to an error.

`guild_settings.event_banner_url` is an image shown under the roster, set with
`/config banner`. One per guild rather than one per raid tier, because the bot has no
concept of a tier, only a free-text title, so there is nothing to key a per-raid image
off yet. The service refuses anything that is not an https URL: Discord renders nothing
else, and storing an http link would produce a card with silently missing artwork.

Class and spec icons are Discord **application** emoji, owned by the app rather than a
guild, so one upload covers every server. `make icons` publishes a directory of PNGs and
the bot matches them by name at startup.

Spec icons are keyed `class_spec`, and the class half is load-bearing: spec names are not
unique. Frost is a Mage and a Death Knight, Holy is a Paladin and a Priest, Restoration
is a Druid and a Shaman. A character whose spec has no icon falls back to their class
icon, and one with neither renders as a plain name, so a partial upload is useful rather
than broken.

### Who gets pinged

`guild_settings.event_mention_role_ids` holds the roles a new event message pings,
typically the guild's raider and trial roles. `/config mentions` sets them from a role
picker; the dashboard is the eventual editing surface and reads the same field.

The ping is **message content, not embed content**. A mention inside an embed renders
as a role name and notifies nobody, which would be exactly wrong for the one message a
guild wants its raiders to see. `allowed_mentions` then names the permitted role ids
explicitly, so a raid titled `@everyone last chance` cannot ping the server.

An empty list is a guild choosing to ping nobody, not a missing setting, which is why
the API always returns the field even when it is empty.

### Where the message is posted

`/config channel` sets a per-guild events channel, stored service-side in
`guild_settings.events_channel_id` and read on every event creation. A guild that has
not set one gets the channel the create command was run in, which is what a guild
trying the bot for the first time expects. If the settings read fails, the bot posts in
the current channel rather than refusing: a message in the wrong place can be moved, a
failed create loses the event.

Everything downstream follows the channel the message actually landed in, since the bot
writes it back via `PATCH /api/events/{id}`. Edits go there, and so do the
`SIGNUP_DEADLINE`, `COMP_NAG` and `LATE_REQUEST_FILED` posts. Changing the setting does
not move events that were already posted.

### Where each field comes from

This matters more than it looks, because two different sources feed one embed:

- **Tanks, Healers, Melee, Ranged, Bench** come from the **comp**, not from signup
  statuses. Bench membership lives on `comp_slots.is_bench` and is decided fresh by
  every lock.
- **Late, Tentative, Declined, Absent** come from **signup status**, which is what the
  raider self-reported.

Signup rows and comp slots both carry their character inline, name and role menu
included, so the embed builder needs no roster lookup of its own and takes no roster
argument. A character deleted between the two reads comes back without a summary, and
renders as "unknown raider" rather than as an invented name.

Before a comp exists, the role fields show signups grouped by what each character
*can* play, with a note that nothing is assigned yet. After a lock, they show
assignments.

### Display rules

- **Times use Discord timestamps** (`<t:unix:F>` for absolute, `<t:unix:R>` for
  relative) so every member sees their own timezone. Never render a formatted time
  string bot-side.
- **Flex players carry a labelled role marker**, and the label flips with context.
  Listed under the role they ranked first, the spares read `Danthrax (offspec melee)`.
  Listed under anything else, which is what a comp lock does when it moves someone, it
  reads `Danthrax (main tank)` instead: the salient fact is that they are playing
  off-spec and where their main is. An unlabelled role name said neither, and after a
  lock it implied the wrong one. This is the visible payoff of the multi-role model and
  should not be dropped for tidiness.
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
- **5 buttons per action row, 5 rows per message.** The five answers fill the first row
  exactly and Withdraw takes the second, since it is not an answer but a way to take one
  back. A sixth in one row is refused as the whole message, not as the extra button.
- **Select menus allow 25 options.** Relevant for character selection when a user has
  many alts, not for role selection.

If a roster genuinely exceeds what an embed can show, the embed shows counts and a
dashboard link rather than a truncated mess.

---

## 4. Interaction flows

### 4.1 Signup, returning user

**A returning raider is signed up on the first click.** Their role menu lives on the
character and was set the first time; asking them to confirm it again answers nothing.

This is also forced by the platform. A select menu only produces an interaction when
the selection *changes*, so a menu offered with the raider's existing roles already
ticked cannot be submitted: there is nothing to change. Offered as a signup step it was
a dead end, and the only way through was to pick a role you do not play and then change
back. The role select is now shown only to raiders who have no menu yet, and
`/character roles` is where an existing one is edited.

1. User clicks `Sign up`.
2. Bot defers immediately (ephemeral).
3. Bot fetches the user's characters and registered roles from the service.
4. **One character:** straight to the answer. A raider whose role menu already exists is
   signed up on the spot; one without a menu yet gets the ephemeral role select,
   multi-select, ordered by priority.
   **Multiple characters:** character select first, then the same. The main sorts first
   and the alts are labelled, since the main is the usual answer.
5. User confirms. Bot POSTs the signup.
6. Bot edits the original event message with the updated embed.
7. Ephemeral confirmation to the user, auto-dismissed.

Target: two clicks for the common case, and the character select does not change that:
it only appears for raiders who own the ambiguity.

Every write that names a character asks the same question the same way. `Tentative`,
`Late`, `Decline`, `Absent`, `Withdraw`, and `/character roles` all reach the select when
the raider has more than one character, and all skip it when they have one. A raider
signed up on an alt has to be able to withdraw that signup, so exempting the one-click
answers would have left a signup nothing could take back.

`Late` is the exception in ordering. Its modal cannot be opened from a deferred
interaction and the bot cannot count a raider's characters without asking the service
first, so the modal comes first and the character select follows it. The arrival time
rides in the select's `custom_id`, since the pick arrives as a fresh interaction that
remembers nothing of the modal.

No option in the character select is ticked by default, for the same reason the role
select is not pre-ticked on the signup path: Discord sends no interaction when a
selection does not change, so a ticked option is one nobody can choose.

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

### 4.4 Tentative, Decline, and Absent

Single click each. None asks for a reason; an optional note can come later from the
dashboard. All three update the embed immediately.

`Absent` and `Decline` are both a no, and the difference is scope. `Decline` answers
this raid. `Absent` says the raider is away for a stretch, so it is counted apart in
the footer: four declines on one night is a scheduling problem, four absences is half
the roster on holiday. Both are the raider's to set. `NO_SHOW` is the raid lead's
alone and the bot never sends it.

### 4.5 Late

A button, and the only signup control that opens a modal. `LATE` carries a `late_until`
timestamp, which is the thing that makes the status actionable, and no click can supply
one. The button therefore responds with the modal directly rather than deferring, which
is hard rule 1's stated exception. The cost is that a raider with no characters
registered only learns so on submit.

The arrival time is parsed with the same reader as a raid start time, anchored on the
event's `starts_at` rather than on now. A raider answering at lunchtime who types
`20:30` means half an hour into tonight's raid, and `01:00` against a 22:00 pull means
the following morning. A time at or before the start is refused: that is not late.

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
4. Bot updates the public embed.

The lock writes immediately rather than waiting for a confirmation step. The board is
already persisted by the time the service answers, so a confirm button would offer to
undo something that had happened, which is worse than showing what did. Locking again
recomputes from scratch, so the fix for a bad board is another lock.

Seated raiders are told their role by `REMINDER_1H`, not at lock time. See open
question 2.

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

The service polls `scheduled_jobs` on a ticker, decides when a reminder is due, and
drains it into a notifications outbox. The bot claims from that outbox on its own
ticker and delivers. It reads no clock and no schedule of its own.

| Trigger | Recipients | Content |
|---|---|---|
| 24h before | Undecided only | Nudge to sign up. Never ping people who already responded. |
| 1h before | Confirmed roster | Their assigned role, invite reminder. |
| Signup deadline | Raid lead | Signups locked, comp needs finalising. |
| Seat given up | Raid lead | A raider left the pool or withdrew after a lock. Names the comps that lost a seat. |
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
GET    /api/guilds/{gid}/settings
PUT    /api/guilds/{gid}/settings          # events channel and timezone, whole-row write

POST   /api/guilds/{gid}/characters
GET    /api/guilds/{gid}/characters
GET    /api/users/{did}/characters
GET    /api/characters/{cid}/roles
PUT    /api/characters/{cid}/roles
PATCH  /api/characters/{cid}
DELETE /api/characters/{cid}

POST   /api/guilds/{gid}/events
GET    /api/guilds/{gid}/events
GET    /api/events/{id}
PATCH  /api/events/{id}
DELETE /api/events/{id}

GET    /api/events/{id}/signups
PUT    /api/events/{id}/signups/{cid}
DELETE /api/events/{id}/signups/{cid}

GET    /api/events/{id}/late-requests
POST   /api/events/{id}/late-requests/{rid}/approve
POST   /api/events/{id}/late-requests/{rid}/reject

GET    /api/events/{id}/comps
GET    /api/events/{id}/comps/{name}
POST   /api/events/{id}/comps/{name}/lock

GET    /api/notifications
POST   /api/notifications/{id}/delivered
```

Every response carries `_links`. The bot renders controls from those links only.

Three things about this list are worth knowing before writing against it, because
earlier drafts of this document got them wrong:

**Signups are keyed by character, not by signup id, and the verb is `PUT`.** The write
is an upsert on `(event_id, character_id)`, so signing up twice is not an error and
needs no read first. There is no `POST /signups`.

**A signup write past the deadline answers `202` with a late request, not an error.**
The player is not refused, their write becomes something a raid lead can act on. A
`DELETE` past the deadline files a request carrying `DECLINED` rather than deleting.

**The notification routes take no actor headers.** They sit behind the shared service
key alone, and one claim spans every guild. Every other route needs
`X-Actor-Discord-Id`, `X-Actor-Guild-Id`, `X-Actor-Roles` and `X-Actor-Guild-Admin`
alongside the key.

Manual comp saving and AUTO/MANUAL conversion are dashboard concerns and are
deliberately absent from this list.

---

## 10. Open questions

1. **One bot instance per guild, or shared?** Shared for the hosted instance,
   single-guild for self-hosters. The code should not assume either.
2. **How does a comp lock DM the seated roster?** It cannot today. A comp slot names a
   character, and no response exposes the Discord user behind one. `REMINDER_1H` does
   carry a `discord_id` and does tell each raider their assigned role, so nobody is
   left uninformed, they hear an hour out rather than at lock. Closing the gap means a
   `discord_id` on the character summary.
3. **Should `/character remove` ship?** `DELETE /api/characters/{cid}` exists and the
   client covers it, but the delete cascades to signups, comp slots and snapshots, and
   nothing in Discord says so loudly enough yet.

**Resolved since the last draft:**

- **Character identity across guilds.** `users` is unique on
  `(discord_id, discord_guild_id)`, so the same person in two guilds is two user rows
  with their own characters. The bot never needs to reconcile them.
- **Push or poll for reminders?** Poll. The service drains `scheduled_jobs` into a
  notifications outbox, and the bot claims from it. Claims lease for five minutes and
  delivery is at-least-once, so a bot that dies mid-batch resends rather than losing
  the batch.
- **Where does the raid lead role get configured?** `PUT
  /api/guilds/{gid}/raid-lead-roles`, guild admin only. A guild with no mapping treats
  Discord admins as raid leads and nobody else, so a fresh install works before anyone
  opens the dashboard. A `/config` command is therefore useful but not blocking.
- **Does `/comp lock` need a comp name?** No. The bot uses one well-known name,
  `main`, and the service creates the comp on first lock. Extra named comps are a
  dashboard concern.
- **Where do raid lead controls live?** Slash commands, not buttons on the event
  message. A message's components are identical for everyone who can see it, so a lock
  button could not be gated on the `lock` link the way hard rule 6 requires. A
  command's reply is ephemeral, which is where that gating can honestly happen.

---

## 11. v0.1 scope

Everything above is the target. v0.1 is narrower, and this is what is built:

- `/raid create`, `/dungeon create`, `/character add`, `/character roles`,
  `/character main`, `/config channel`, `/config timezone`, `/config mentions`,
  `/config banner`, `/help`
- Signup, Tentative, Late, Decline, Absent, Withdraw buttons, Late via a modal
- Multi-role select, ordered by priority, existing picks pre-ticked
- Character select on every write that names one, for raiders with more than one
- Event embed with role fields and counts, redrawn on a one-second debounce
- `/comp show` and `/comp lock` on AUTO comps, with advisories and reasons surfaced
- Every notification kind delivered from the outbox

Deferred: `/event edit`, `/event cancel`, `/audit`, `/absence` (the button answers one
raid; the command would answer a date range across many),
`/roster`, `/character remove`, waitlist promotion, the late-request approve and reject
surface, and any manual comp editing from Discord.
