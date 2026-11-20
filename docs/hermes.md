# Local Hermes

Hermes runs in Docker with the repository mounted at `/workspace`. The host does not need a Hermes installation.

The development image also installs Graphify. Graphify is used by Hermes as a repository orientation and code/document relationship tool; it is not part of the Go application or its Windows executable.

## Start

From the repository root:

```bash
docker compose -f infra/local/docker-compose.hermes.yaml build
docker compose -f infra/local/docker-compose.hermes.yaml up -d
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

The Hermes agent is instructed to use the installed Graphify skill for repository orientation. Graphify is a development tool only; it is not included in the Go application or its Windows executable.

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

Implementation tasks use the sequential Developer → Tester → Reviewer workflow. The Reviewer is read-only and routes fixes back to Developer or Tester. No commits, pushes, pull requests, Slack, GitHub Project, DynamoDB, Terraform, or delivery infrastructure are part of this local setup.

## Configuration

The tracked Hermes configuration is `.agents/hermes/config.yaml`. Provider credentials and Hermes runtime state stay in the host-mounted data directory and must not be committed.
