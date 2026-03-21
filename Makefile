
.PHONY: generate build db-reset db-up db-down test seed

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

# seed uploads procedurally generated assets to a running local instance.
seed:
	go run ./cmd/seed

generate:
	go generate ./...
