# Local Hermes

Hermes runs in Docker with the repository mounted at `/workspace`. The host does not need a Hermes installation.

The development image also installs Graphify. Graphify is used by Hermes as a repository orientation and code/document relationship tool; it is not part of the Go application or its Windows executable.

## Start

From the repository root:

```bash
just hermes-setup
```

The equivalent lifecycle commands are:

```bash
just hermes-build
just hermes-up
just hermes-status
```

The container automatically trusts `/workspace` and starts the Hermes gateway. Persistent Hermes credentials and session data are stored in `${HERMES_DATA_DIR:-$HOME/.hermes}`.

Install Graphify's Hermes project skill once after the first build:

```bash
docker compose -f infra/local/docker-compose.hermes.yaml exec hermes graphify install --project --platform hermes
```

Use Graphify before implementation or review. It builds/refreshes its local project knowledge as needed:

```bash
docker compose -f infra/local/docker-compose.hermes.yaml exec hermes graphify query "Where are the scenario, parser, and report boundaries?"
docker compose -f infra/local/docker-compose.hermes.yaml exec hermes graphify explain "ScienceReportScenario"
docker compose -f infra/local/docker-compose.hermes.yaml exec hermes graphify path "Scenario" "ReportRun"
```

## GitHub Projects MCP

Hermes uses its native MCP client with GitHub's official remote MCP server. It
does not use the GitHub CLI or a task manifest. Because project #2 is owned by
your GitHub user account, use a classic personal access token: GitHub currently
does not support user-owned Projects with fine-grained tokens. For this public
repository, grant only `project` and `public_repo`; use `repo` instead of
`public_repo` only if the repository is private. Store the token in the Hermes
profile environment at `~/.hermes/assistent/.env`:

```text
GITHUB_PERSONAL_ACCESS_TOKEN=...
```

The configured MCP connection enables the `context`, `projects`, `issues`,
`repos`, `git`, and `pull_requests` toolsets. Verify the connection with:

```bash
just hermes-setup
```

GitHub MCP authentication and Hermes' AI authentication are separate. Connect
Hermes to OpenAI Codex once using its OAuth flow:

```bash
just hermes-connect-codex
```

The project configuration selects `openai-codex` as Hermes' model provider.
This uses Hermes-managed Codex OAuth credentials in the Hermes profile; it is
not the GitHub token and does not use `OPENAI_API_KEY`. If you want to use an
OpenAI API key instead, the provider would be `openai-api`, not
`openai-codex`.

Start the full project workflow with:

```bash
just hermes-tasks
# Or choose an explicit issue range:
just hermes-tasks-range 7 16
```

This starts the unattended delivery loop. The runner uses a fresh bounded Hermes
one-shot session for each Developer, Tester, Reviewer, review-fix, and the
dedicated non-delegating Delivery agent. It keeps only the reviewer handoff between phases, preventing one parent
conversation from accumulating the entire repository and all prior tool output.
Each phase still re-reads the required documents and uses Graphify before source
exploration. The runner commits and pushes an approved branch, creates and merges
the linked pull request, marks the project item Done, then switches to `main` and
pulls before starting the next issue. Phase logs are written to `/tmp/hermes-delivery-*`.

GitHub MCP is configured with full trust and the task session runs with Hermes
YOLO mode enabled, so this command does not wait for approval buttons. This is
deliberately autonomous and should only be run when changes, merges, and project
updates are authorized.

The Hermes agent is instructed to use the installed Graphify skill for repository orientation. Graphify is a development tool only; it is not included in the Go application or its Windows executable.

The same compact runner can be started from the dashboard. Open a new chat and
send this request:

```text
Run ./scripts/hermes-delivery-loop.sh 8 16 in the terminal and keep the terminal output visible. Do not delegate the workflow yourself; the script starts fresh Hermes sessions for each phase. Do not ask for approval.
```

When launched from inside the Hermes container, the script invokes the local
Hermes binary directly. When launched from the host, it uses Docker Compose.

## Interactive shell

```bash
docker compose -f infra/local/docker-compose.hermes.yaml exec hermes sh
```

## Logs and stop

```bash
docker compose -f infra/local/docker-compose.hermes.yaml logs -f hermes
docker compose -f infra/local/docker-compose.hermes.yaml down
```

## Repository workflow

The mounted repository provides:

- `AGENTS.md`;
- `ARCHITECTURE.md`;
- `SYSTEM_DESCRIPTION.md`;
- `database.md`;
- `TESTING.md`;
- `.agents/skills/`;
- `.agents/hermes/config.yaml`.

By default, Hermes stores its profile data in `~/.hermes/assistent`. Set
`HERMES_DATA_DIR` if a different profile location is needed.

The Hermes container is restricted to the explicitly mounted project workspace
and Hermes data directory. Its image filesystem is read-only, all Linux
capabilities are dropped, privilege escalation is disabled, and no Docker
socket or general host directory is mounted. The project workspace remains
writable because that is where Hermes is expected to make code changes.

The container still has network access because Hermes may need to contact its
configured model/provider services. This is separate from host filesystem
access. Do not mount GitHub tokens, SSH keys, or credential directories into
the container.

Implementation tasks use the sequential Developer → Tester → Reviewer workflow. The Reviewer is read-only and routes fixes back to Developer or Tester. After approval, the parent orchestrator commits, pushes, opens and merges the linked pull request, marks the GitHub Project item Done, and synchronizes the local checkout with `main` before continuing. No Slack, DynamoDB, or Terraform infrastructure is involved.

## Configuration

The tracked Hermes configuration is `.agents/hermes/config.yaml`. Provider credentials and Hermes runtime state stay in the host-mounted data directory and must not be committed.
