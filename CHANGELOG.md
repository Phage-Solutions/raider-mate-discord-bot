# Changelog

Notable changes to raider-mate-discord-bot. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and versions follow
[Semantic Versioning](https://semver.org/spec/v2.0.0.html) without a `v` prefix.

The release workflow reads the section matching the pushed tag and uses it as the
GitHub Release body. A tag with no section here fails the release before anything is
published.

Sections are `Added`, `Changed`, `Deprecated`, `Removed`, `Fixed`, `Security`.

## [Unreleased]

## [0.6.1] - 2026-08-20

### Changed

- Redraw requests from the service now wait out the same one-second window a button
  click does, rather than editing the message the moment each one lands. The service has
  started queueing a redraw per signup, and without this a raid of twenty answering at
  once would be twenty edits of the same message against a per-channel rate limit.

## [0.6.0] - 2026-08-20

### Added

- **Events created in the dashboard get their signup sheet posted here.** The bot picks
  the new event up from the outbox, posts the same card `/raid create` posts, pings
  whichever roles the guild configured, and tells the service where it landed. Without
  that last step the event would have no message for a redraw to edit and no channel for
  a reminder to speak in. Needs the raider-mate-service release that queues the
  announcement.

## [0.5.0] - 2026-08-20

### Added

- `/character remove` unregisters a character. It asks for confirmation first, because
  the character's signups and comp slots go with it and none of it comes back.

## [0.4.0] - 2026-08-19

### Changed

- Signups now close an hour before the raid by default, rather than a day. Pass
  `signups_close` on `/raid create` or `/dungeon create` to set your own.
- `Late` and `Absent` can be answered right up to the pull, even once signups have
  closed. Both report what is happening on the night, so they no longer turn into a
  request a raid lead has to sign off. Needs the raider-mate-service release that
  carries the same change.

### Removed

- The "comp is not locked" nag two hours before a raid. Locking a comp is optional, so
  the bot no longer chases raid leads about it.

## [0.3.2] - 2026-08-16

### Added

- `/raid create` and `/dungeon create` take a `reminder` option: how many minutes before
  the start everyone signed up is reminded. Leave it out for the server's default, or
  pass `0` for no reminder on that event.
- `/config reminder` sets that default and how the reminder arrives: a ping in the
  events channel, a DM to each raider, or both. Server admins only. A server that sets
  nothing gets a ping 30 minutes out.

### Changed

- The reminder before an event now reaches everyone whose signup says they are coming
  (confirmed, late or tentative), whether or not the comp is locked and whether or not
  they got a seat in it. It used to reach only the raiders holding a seat, so anyone
  left out of a locked roster heard nothing. Signing up on several characters is still
  one reminder.
- That reminder now goes to the events channel as a ping by default, rather than a DM
  to each raider. The DM was easy to miss ten minutes before an invite. `/config
  reminder` puts DMs back, alone or alongside the ping. The ping links the event message
  instead of naming anyone's role; the DM still names it.
- Requires a `raider-mate-service` release carrying the `REMINDER_PRE_EVENT` job and the
  reminder settings. This release still delivers the old `REMINDER_1H` notification, so
  the bot can be updated first; that fallback goes away in a later release.

## [0.3.1] - 2026-08-16

### Added

- `make icons` / `UploadIcons` now accepts `.webp` alongside `.png`. Discord's emoji
  upload takes JPEG, PNG, GIF, WebP and AVIF; only PNG was wired up before, so a WebP
  icon set silently uploaded nothing. `.gitignore` now excludes `assets/icons/*.webp`
  too, for the same reason it already excluded `*.png`.

### Changed

- The event embed now shows the signup tally ("N in, M absent") as a bolded line
  between two dividers, right under the dates. It used to sit in the small footer text
  next to the event id, which made it easy to miss when scanning for whether a raid
  had enough sign-ups.

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
