# Changelog

Notable changes to raider-mate-discord-bot. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and versions follow
[Semantic Versioning](https://semver.org/spec/v2.0.0.html) without a `v` prefix.

The release workflow reads the section matching the pushed tag and uses it as the
GitHub Release body. A tag with no section here fails the release before anything is
published.

Sections are `Added`, `Changed`, `Deprecated`, `Removed`, `Fixed`, `Security`.

## [Unreleased]

## [0.3.0] - 2026-08-16

### Added

- The bot now pushes the guild's Discord channel and role list to the service
  whenever it changes (channel or role created/renamed/deleted) and on every gateway
  reconnect, so the dashboard has something to build a picker from for the events
  channel and the mention roles. No new command and nothing a raider sees: this is
  bot-to-service only. Requires a `raider-mate-service` release carrying the
  `discord-channels`/`discord-roles` guild endpoints.

## [0.2.0] - 2026-08-16

Run `make register` after deploying this one. `/character main` is a new command
definition, and Discord will not show it until the definitions are published.

### Added

- Raiders with more than one character are asked which one they mean. `/character roles`
  and every signup control on an event message (Sign up, Tentative, Late, Decline,
  Absent, Withdraw) now open a character select first. Raiders with a single character
  see no extra step and answer in the same one click as before. Before this, everything
  went to the main and an alt could not be reached at all.
- `/character main` moves the main flag to another of your characters. It lists your
  alts, and picking one demotes the character that held the flag.
- Characters that are not your main are marked `[Alt]` wherever their name is shown: the
  event roster, the bench, the status fields, the comp board, and the bot's own replies.
- Raider names in the event roster link to their Raider.IO page, in the role columns
  and on the bench, before and after a comp lock. Only the name is clickable; the item
  level and the markers after it are not. Needs raider-mate-service 0.2.0 or newer,
  which is where the link comes from; against an older service the names render plain
  as they always did.

### Changed

- A roster column fits fewer raiders before it truncates to "and N more". Each link
  spends around fifty characters of Discord's 1024-character field limit, and the limit
  did not move.

### Fixed

- `/character roles` reported an error after saving. It wrote the role menu and then
  tried to sign the raider up for an event that does not exist, since the command has no
  event behind it. The menu had actually saved both times, which is what made it
  confusing.

## [0.1.2] - 2026-08-16

### Fixed

- A panic in an interaction handler no longer kills the bot. discordgo dispatches each
  handler in its own goroutine, so any nil dereference below `onInteraction` unwound
  past `main` and ended the process; the image is distroless, so nothing restarted it
  in place, and the runtime's trace went to stderr as plain text rather than through
  the JSON logger, which is why a crash like that left nothing in the log stream. The
  panic is now logged with its stack, and the raider gets a reply instead of a spinner
  that expires.
- A panic while delivering a notification no longer ends the delivery loop or the
  process. The recover is per drain, so one row the bot cannot render costs that batch
  and not every reminder after it; the claim lapses and the next tick retries. A panic
  in the outbox stream drops the bot back to its ticker, where it was before the stream
  existed.

## [0.1.1] - 2026-08-16

### Added

- `GET /healthz` on `PORT` (default `8080`), for platforms whose health check needs a
  port to watch. The bot takes no other inbound traffic; this answers 200 once the
  gateway connection is open.

## [0.1.0] - 2026-08-16

Initial release: raid and dungeon events, multi-role signups, comp lock, and reminder
delivery against `raider-mate-service`.

### Added

- `/event raid create` and `/event dungeon create`: title, start time, signup
  deadline, difficulty, and per-role signup counts for a raid.
- `/character add` and `/character roles`, for registering a character and editing
  which roles it can play.
- `/comp show` and `/comp lock`. Both are slash commands rather than buttons, since a
  message's components are the same for every viewer and a lock button could not be
  gated on the lock link the way hard rule 6 requires; a command's ephemeral reply is
  where that gating can honestly happen.
- `/config channel`, `/config banner`, `/config mentions`, `/config timezone`, and
  `/help`.
- Event messages with signup buttons (Signup, Tentative, Decline, Late, Withdraw) and
  role-select components, edited in place via the stored `message_id` and never
  reposted.
- `Absent` button on event messages. One click, no role menu, for a raider who is out
  for a stretch rather than declining this one raid.
- `Late` button, which opens a modal for the arrival time. The time is read against
  the raid's start, so `20:30` from someone answering at lunchtime means half an hour
  into tonight's raid, and a time at or before the pull is refused.
- Notification delivery from the service's outbox: claim-based polling plus a
  Server-Sent Events stream, redialled after a five-second backoff on disconnect, so
  nothing is missed while the connection is down. Covers `REMINDER_24H`,
  `REMINDER_1H`, `SIGNUP_DEADLINE`, `COMP_NAG`, and `COMP_SLOT_DROPPED`, which names
  the comps a raider's seat came out of after a lock.
- `UploadIcons`, publishing every PNG in a directory as an application emoji named
  after its file. Application emoji belong to the bot rather than a guild, so one
  upload covers every server it is in, and a self-hoster running their own
  application uploads their own set.
- `allowed_statuses` is read off signup responses. The bot's buttons are message-wide
  and cannot be gated per viewer, so nothing here acts on it yet.

### Changed

- Absent raiders get their own embed field and their own footer tally, counted apart
  from declines. Late and Absent both read under the roster, as what raiders said
  about themselves rather than as part of what the comp did with them.
- Event messages use two button rows. The five answers fill the first, Withdraw takes
  the second.
- The signup-deadline summary names absences and still skips zeroes, now that the
  service sends a count for every status.
