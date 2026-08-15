.PHONY: run register icons test lint fmt docker

# Local config lives in .env, which the binary reads itself (cmd/bot/dotenv.go). That
# differs from raider-mate-service, where make does the exporting: this bot is started
# from an IDE as often as from a terminal, and one mechanism that works everywhere beats
# a run configuration that needs a marketplace plugin. Real environments set the
# variables directly and those always win over the file.

run:
	go run ./cmd/bot

# Registration is separate from run on purpose: Discord allows 200 command creates per
# day per guild, and re-registering on every restart burns through that during a normal
# afternoon of development.
register:
	go run ./cmd/bot -register

# Icons are application emoji, uploaded once and used in every guild. Name each file
# after what it shows: warrior.png for a class, mage_frost.png for a spec.
icons:
	go run ./cmd/bot -upload-icons ./assets/icons

test:
	go test ./...

lint:
	go tool golangci-lint run

fmt:
	gofmt -l -w .

docker:
	docker build -t raider-mate-bot .
