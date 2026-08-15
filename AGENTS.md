# AGENTS.md: raider-mate-bot

Discord bot for Raider Mate, built with `discordgo`. This repo is a thin client of
`raider-mate-service`'s HTTP API. It holds no schema, no domain logic, no direct
database access. If a change here needs a new rule about how signups or comps work,
that rule belongs in `raider-mate-service`, not here.

Licensed AGPLv3. Free to self-host, monetised via a hosted instance.

Shared conventions (licensing, writing style, the "keep in sync" note) are duplicated
across raider-mate-service, raider-mate-bot, and raider-mate-dashboard on purpose.
The canonical copy of the shared sections lives in raider-mate-service's AGENTS.md.
If you need to change a shared section, edit it there first, then copy the edit here.

The Discord surface (commands, embeds, interaction flows, the implied API contract)
is specified in [docs/design.md](docs/design.md). The full domain design lives in
`raider-mate-service/docs/design.md`. The writing style rules live in
[docs/style.md](docs/style.md). Read them when the task touches those areas. Do not
load them by default.

## Stack

- Go, `discordgo`.
- No database driver, no ORM. All state lives behind the service API.
- HTTP client against `raider-mate-service`.

**One deliberate divergence from the service.** `raider-mate-service` reads no dotenv
and leaves `.env` to its Makefile. This repo reads one, in `cmd/bot/dotenv.go`, because
the bot is started from an IDE as often as from a terminal, and the alternatives were a
marketplace plugin as a prerequisite or the Discord token in a tracked run
configuration. A real environment variable always wins over the file, and a missing
file is not an error, so containers and CI behave exactly as they did before.

The author is experienced in Kotlin/Spring and new to Go. Write idiomatic Go, not Java
in Go syntax. When an idiom differs from the JVM equivalent, note it in one line.

## Commands

```
make run       # start the bot locally, pointed at a running raider-mate-service
make register  # publish slash commands, then exit
make test      # go test ./...
make lint      # golangci-lint run
make fmt       # gofmt -l -w .
make docker    # build the image, rootless distroless
```

Registration is separate from `run` because Discord allows 200 command creates per day
per guild, and re-registering on every restart spends that during one afternoon. Run it
when the command definitions change.

Embed golden files live in `internal/discord/testdata`. Rewrite them with
`go test ./internal/discord -update`, then read the diff before committing it.

Always run `make test` and `make lint` before declaring work finished.

There is no `make test-integration` here. This repo has no database, so there is
nothing for testcontainers to stand up.

## Hard rules

Violating these produces broken behaviour, not just untidy code.

1. **Defer Discord interactions within 3 seconds.** Respond with
   `DeferredChannelMessageWithSource` first, then call the service API, then edit.
   The one exception is a flow that must open a modal, which cannot be done from a
   deferred interaction. See docs/design.md section 4.2.
2. **Edit event messages via the stored `message_id`.** Never repost. Users pin them.
3. **This repo holds no business logic.** It parses interactions, calls the service
   API, renders embeds. If it computes a comp, validates a signup, decides who is
   benched, or touches Postgres, that is a bug: those belong in
   `raider-mate-service`.
4. **Avoid privileged Discord intents.** Interactions and guild members only. Needing
   message content or presence complicates verification past 100 servers.
5. **Treat the service API as an external contract.** Do not assume a field, a link,
   or a status value exists because you remember it from the schema doc. Check the
   actual API response, since the service changes on its own release cycle.
6. **Use HATEOAS links from API responses to decide what actions to offer**, rather
   than hardcoding which buttons appear for which role. If the API did not return a
   `bench` link for this caller, do not render a bench button.
7. **Bench is not a signup status.** It lives on the comp, not on the signup. Never
   send a `BENCH` status to the service; it was dropped from the enum. Read bench
   membership from the comp response.
8. **Use Discord timestamps (`<t:unix:F>`)** so event times render in each user's
   local zone. Never format a time string bot-side.
9. **Do not autocommit and push, at all.** Leave changes staged, uncommitted, for the
   author to review, commit, and push themselves.

## Structure

```
cmd/bot            main, config from the environment, hand-wiring
internal/discord   interaction handlers, embed rendering, component builders
internal/client    typed HTTP client for raider-mate-service, incl. HATEOAS parsing
```

Never create `service/`, `repository/`, `dto/`, `utils/`, or `impl/` packages.
`discordgo` types stay inside `internal/discord`. Never let them leak into
`internal/client` or beyond.

## Go conventions

- Wire dependencies by hand in `main()`. No DI framework.
- Errors are values. Wrap with context: `fmt.Errorf("posting embed for event %s: %w",
  eventID, err)`. No panic outside startup failure.
- `context.Context` is the first parameter of every I/O function, honouring Discord's
  interaction deadline.
- Interfaces are small (1 to 3 methods) and declared by the consumer. Never
  `FooRepository` plus `FooRepositoryImpl`.
- Accept interfaces, return structs. No global state, no `init()` side effects.
- Ask before adding a dependency. Standard library first.

## Principles

Priority order when they conflict: **KISS > YAGNI > DRY > SOLID**.

- **KISS.** Prefer the boring solution. A switch beats a strategy pattern.
- **YAGNI.** If there is one implementation, there is no interface for it yet.
- **DRY.** De-duplicate knowledge, not text. Rule of three.
- **SOLID.** ISP matters most in Go: interfaces belong to the consumer.
- **DDD.** Use guild vocabulary: `Raider`, `Bench`, `Comp`, `Lockout`, `RaidLead`.
  Never `Participant`, `Entity`, `Item`.

Note the vocabulary change: the service calls the privileged user a **raid lead**, not
an officer. Match it.

## Behaviour

- **Do not assume.** State assumptions. Where a request has several readings, present
  them instead of silently choosing. When unclear, stop and ask.
- **Minimum code that solves the problem.** No speculative features, abstractions,
  configurability, or error handling for impossible cases.
- **Surgical changes.** Do not improve adjacent code, refactor working things, or
  reformat. Remove only the orphans your own change created. Every changed line traces
  to the request.
- **Verifiable goals.** "Fix the bug" becomes "write a failing test, then make it
  pass". State a short plan with a verify step per item for multi-step work.
- Small commits, one concern each. Imperative lowercase subject, no trailing period.

## Writing style

**No em dashes.** No litanies of three. No emoji in code, comments, or commits. No
banned filler: `robust`, `seamless`, `comprehensive`, `leverage`, `delve`,
`ensure that`, `it's worth noting`. Comment why, not what. Full rules in
[docs/style.md](docs/style.md).

Emoji in Discord embeds and button labels are a separate matter. Those are product
copy read by players, not code, and a role marker or a button glyph is often the
clearest option. The ban applies to source, comments, commits, and logs.

## Testing

**Unit tests, standard library `testing`, no containers.** This repo has no database,
so everything is a unit test. The value is concentrated in three places:

**Embed builders as pure functions.** Given signups and a comp, produce an embed
struct. Assert on field counts, ordering, and truncation. This is where the 1024-char
field cap and the 25-field cap get caught. Golden-file tests work well: serialise the
embed to JSON, commit it, diff on change.

**API client against `httptest.Server`.** Fake the service. Assert the client parses
HATEOAS links correctly, handles 4xx without leaking internals into Discord, and does
not invent controls the links did not authorise.

**custom_id round-trips.** Parse and build. Cheap to test, and it catches the stale
version path that pinned messages will eventually hit.

`discordgo` itself is not built for mocking. Do not fight it. Keep handlers thin
enough that the logic worth testing sits outside them.

## Build order

1. Typed HTTP client for the service API, HATEOAS-aware
2. Event creation, signup buttons, role select menus
3. Embed rendering and editing via stored `message_id`
4. Comp lock flow, respecting AUTO and MANUAL comp modes
5. Reminder delivery

**v0.1 scope for this repo:** signup buttons with multi-role select, one comp view
rendered from the API, reminder DMs. Nothing here should exist before the matching
service endpoint exists.
