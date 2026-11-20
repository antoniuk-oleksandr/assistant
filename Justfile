default: run

hermes_compose := "docker compose -f infra/local/docker-compose.hermes.yaml"

run:
	go run ./cmd/assistent

build:
	go build -o bin/assistent ./cmd/assistent

test:
	go test ./...

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
	{{hermes_compose}} exec hermes /opt/hermes/bin/hermes chat -q "Use the GitHub Projects MCP tools to inspect https://github.com/users/antoniuk-oleksandr/projects/2/views/1. Complete all 16 Todo issues in dependency order, one at a time. For every issue, follow Developer -> Tester -> Reviewer, create the required local branch and commit, then use the GitHub MCP tools to push the branch and create a pull request linked with Closes #N. Do not stop after one issue. After each pull request is merged, update that project item to Done through the GitHub Projects MCP. Read AGENTS.md and the relevant project documents before coding."
