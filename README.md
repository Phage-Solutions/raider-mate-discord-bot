# Raider Mate bot

Discord bot for Raider Mate, a WoW raid and Mythic+ signup system built around the fact
that raiders play more than one role. This repo renders the Discord surface: slash
commands, event embeds, signup buttons, role selects, reminder DMs.

It holds no schema and no domain logic. Everything it shows comes from
`raider-mate-service` over HTTP, and every control it renders comes from the HATEOAS
links in that service's responses.

## Stack

Go, `discordgo`. No database driver, no ORM, no DI framework.

## Running locally

Requires Go 1.26+ and a running `raider-mate-service`.

```
cp .env.example .env
```

Fill in `DISCORD_TOKEN` from your application in the Discord developer portal, and
`DISCORD_DEV_GUILD_ID` with a test server's id. Then:

```
make register      # publish slash commands to the dev guild, run when they change
make run
```

`SERVICE_API_KEY` must match the key the service was started with.

Command registration is a separate target because Discord allows 200 command creates per
day per guild. Guild-scoped commands appear instantly; leaving `DISCORD_DEV_GUILD_ID`
unset registers globally instead, which is what production does.

## Running from IntelliJ

Shared run configurations live in `.run/`, so they show up in the run dropdown as soon
as the project is opened:

| Configuration | Does |
|---|---|
| `bot` | Runs `cmd/bot`, debugger attachable |
| `bot: register commands` | Publishes slash commands, then exits |
| `all tests` | `go test` across the repo |
| `update embed goldens` | Rewrites `internal/discord/testdata`, read the diff after |

No plugins are needed. The binary reads `.env` itself, so the run configurations hold
no configuration and therefore no secrets, which matters because `.run/` is tracked and
`.env` is not.

## Class and spec icons

Roster lines can carry a spec icon, falling back to a class icon. They are Discord
*application* emoji, owned by the app rather than a guild, so one upload covers every
server the bot is in.

Put PNGs in `assets/icons`, each under 256KB, named after what it shows:

```
warrior.png            a class
mage_frost.png         a spec, prefixed with its class
deathknight_frost.png  a different Frost entirely
hunter_beastmastery.png
```

The class prefix is not optional. Spec names collide across classes: Frost is a Mage and
a Death Knight, Holy is a Paladin and a Priest, Restoration is a Druid and a Shaman.
Names are lowercased with spaces and punctuation removed, matching what Raider.IO
reports.

```
make icons
```

Uploading only the thirteen class icons is a reasonable place to stop: a character whose
spec has no icon falls back to their class. Upload nothing and rosters show plain names.
The bot reads its emoji at startup, so a fresh upload needs a restart.

## Container

```
make docker
docker run --rm --env-file .env raider-mate-bot
```

The image runs as UID 65532 on `gcr.io/distroless/static:nonroot`: no shell, no package
manager, no libc, nothing writable. It needs no volumes and no capabilities, so it drops
straight into a `runAsNonRoot` Kubernetes policy without a securityContext argument.

Registering commands is the same binary with a flag:

```
docker run --rm --env-file .env raider-mate-bot -register
```

## Documentation

`docs/design.md` specifies the Discord surface: commands, embeds, interaction flows, and
the API contract they imply. The full domain design, including the schema and the
assignment algorithm, lives in `raider-mate-service/docs/design.md`.

## License

AGPLv3. Free to self-host. See `LICENSE`, and `CONTRIBUTING.md` for the sign-off
requirement.
