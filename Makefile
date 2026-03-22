
.PHONY: generate build db-reset db-up db-down test seed seed-ddnet-skins seed-ddnet-maps pocketid-provision update-tilesets

DOCKER_COMPOSE = docker compose -f docker/docker-compose.yaml --env-file docker/dev.env

build:
	go build .

syntax:
	go vet ./...
	go build ./...

# db-up starts the database container without wiping data.
db-up:
	$(DOCKER_COMPOSE) up -d --wait

db-down:
	$(DOCKER_COMPOSE) down -v

# db-reset tears down the container, wipes the data volume, and recreates the DB.
db-reset:
	$(DOCKER_COMPOSE) down -v
	rm -rf volumes/postgresql/data
	mkdir -p volumes/postgresql/data
	$(DOCKER_COMPOSE) up -d --wait

test:
	set -a && . docker/dev.env && set +a && go test ./...

# seed-ddnet-skins scrapes the DDNet skin database and uploads all skins.
seed-ddnet-skins:
	go run ./cmd/seed-ddnet-skins

# seed-ddnet-maps fetches maps from the DDNet map repository and uploads them.
seed-ddnet-maps:
	go run ./cmd/seed-ddnet-maps

# pocketid-provision creates the OIDC client + admin user on the local Pocket-ID
# instance and writes the OIDC_CLIENT_ID / OIDC_CLIENT_SECRET into docker/dev.env.
# Run this once after 'make db-up' and before starting the server.
pocketid-provision:
	set -a && . docker/dev.env && set +a && go run ./cmd/provision-pocketid -env-file docker/dev.env

generate:
	go generate ./...

# update-tilesets downloads the latest DDNet external tilesets and license
# into internal/twmap/mapres/ for embedding into the map renderer.
update-tilesets:
	go run ./cmd/update-tilesets -dest internal/twmap/mapres
