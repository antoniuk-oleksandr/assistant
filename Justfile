default: run

hermes_compose := "docker compose -f infra/local/docker-compose.hermes.yaml"

run:
	go run ./cmd/assistent

build:
	go build -o bin/assistent ./cmd/assistent

test:
	go test ./...

coverage:
	go test ./... -coverpkg=./cmd/assistent,./internal/... -coverprofile=coverage.out
	go tool cover -func=coverage.out
	go tool cover -html=coverage.out -o coverage.html

hermes-build:
	{{hermes_compose}} build

hermes-up: hermes-build
	{{hermes_compose}} up -d

hermes-setup: hermes-up
	{{hermes_compose}} exec -T hermes graphify install --project --platform hermes
	{{hermes_compose}} exec -T hermes /opt/hermes/bin/hermes mcp test github

hermes-status:
	{{hermes_compose}} ps

hermes-connect-codex:
	{{hermes_compose}} exec hermes /opt/hermes/bin/hermes auth add openai-codex

hermes-tasks:
	./scripts/hermes-delivery-loop.sh 8 16

hermes-tasks-range start end:
	./scripts/hermes-delivery-loop.sh {{start}} {{end}}
