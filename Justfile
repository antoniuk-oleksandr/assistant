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
	{{hermes_compose}} exec -T hermes /opt/hermes/bin/hermes chat --yolo --oneshot --quiet --max-turns 500 -q "Run the autonomous project delivery loop for https://github.com/users/antoniuk-oleksandr/projects/2/views/1. This is an authorized unattended run: do not ask me for approval. Use the GitHub Projects, issues, repository, git, and pull_requests MCP tools for GitHub operations; do not use gh. Re-read the project at the start of every iteration and select exactly one current Todo item in dependency order. Never assume there are 16 items or reuse a cached list. Before each item, run git switch main and git pull --ff-only origin main, verify the working tree is clean, create a branch from main, and inspect the issue and its dependencies. Read AGENTS.md, ARCHITECTURE.md, SYSTEM_DESCRIPTION.md, database.md when persistence is involved, TESTING.md for tests, and the relevant .agents/skills/ files. Execute the sequential Developer -> Tester -> Reviewer workflow by calling Hermes built-in delegate_task exactly one child at a time: load the hermes-dev skill for Developer, hermes-tester for Tester, and hermes-reviewer for Reviewer. These are project skills, not Hermes profiles; do not search for or require profiles named hermes-dev, hermes-tester, or hermes-reviewer. The Developer changes production code, the Tester changes tests only, and the Reviewer is read-only. Apply up to three review-fix cycles; route production_fix to Developer and test_fix to Tester. Do not claim completion until the Reviewer returns APPROVED or reports a human blocker. For an approved item, run the required checks, commit the complete item change with the required issue-aware commit message, push the branch, open a pull request targeting main with Closes #N in the body, and merge it using GitHub MCP. Verify the PR is merged and closed, then update that exact project item to Done. After the project update, switch to main and git pull --ff-only origin main locally, verify the merge is present, and only then begin the next iteration. Continue until a fresh project read shows no Todo items. If a genuine blocker occurs, stop and report the exact item, operation, and error rather than silently skipping it."
