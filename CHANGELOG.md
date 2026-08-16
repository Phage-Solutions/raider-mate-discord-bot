# Changelog

Notable changes to raider-mate-discord-bot. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and versions follow
[Semantic Versioning](https://semver.org/spec/v2.0.0.html) without a `v` prefix.

The release workflow reads the section matching the pushed tag and uses it as the
GitHub Release body. A tag with no section here fails the release before anything is
published.

Sections are `Added`, `Changed`, `Deprecated`, `Removed`, `Fixed`, `Security`.

## [Unreleased]

### Added

- `Absent` button on event messages. One click, no role menu, for a raider who is out
  for a stretch rather than declining this one raid.
- `Late` button, which opens a modal for the arrival time. The time is read against the
  raid's start, so `20:30` from someone answering at lunchtime means half an hour into
  tonight's raid, and a time at or before the pull is refused.
- Delivery of the service's `COMP_SLOT_DROPPED` notification, which names the comps a
  raider's seat came out of after a lock.
- `allowed_statuses` is read off signup responses. The bot's buttons are message-wide
  and cannot be gated per viewer, so nothing here acts on it yet.

### Changed

- Absent raiders get their own embed field and their own footer tally, counted apart
  from declines. Late and Absent both read under the roster, as what raiders said about
  themselves rather than as part of what the comp did with them.
- Event messages use two button rows. The five answers fill the first, Withdraw takes
  the second.
- The signup-deadline summary names absences and still skips zeroes, now that the
  service sends a count for every status.
