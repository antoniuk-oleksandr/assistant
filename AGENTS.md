# Agent Instructions

## Source of truth

Before changing code, read:

1. `ARCHITECTURE.md`;
2. `SYSTEM_DESCRIPTION.md`;
3. `database.md` when the task touches persistence;
4. `TESTING.md` when the task touches tests;
5. the relevant skill under `.agents/skills/`.

These documents define the project architecture and override generic implementation preferences.

## Required workflow

Implementation tasks use the sequential workflow:

```text
Developer → Tester → Reviewer
                    ├── APPROVED → complete
                    └── changes requested → Developer
```

The Developer changes production code. The Tester adds or corrects tests without changing production behavior. The Reviewer is read-only and reports classified findings.

Use no more than three review-fix cycles before asking the human for direction.

## Project constraints

- Go, Uber Fx, Cobra, Zap, Ginkgo/Gomega, and `uber-go/mock`;
- one executable and one `main.go`;
- four layers: Domain, Application, Infrastructure, Presentation;
- ports belong to Domain;
- explicit mappers prevent model leakage between layers;
- SQLite is the runtime state store;
- scenarios are registered and task-bound through the CLI and persisted in SQLite;
- background polling runs every five minutes;
- generated mocks live in `mocks/`;
- shared fixtures live in `tests/support/fixtures`;
- follow SOLID, DRY, KISS, small functions, and small explicit modules.

Do not introduce unrelated infrastructure, delivery systems, message queues, dynamic plugins, or configuration formats that are not described in the project documents.

## graphify

This project has a knowledge graph at graphify-out/ with god nodes, community structure, and cross-file relationships.

When the user types `/graphify`, use the installed graphify skill or instructions before doing anything else.

Rules:
- When graphify-out/graph.json exists, run `graphify query "<question>"` before your first grep, search or source-file read for a codebase question, and put this rule in every subagent prompt that explores code. Read raw files only for lines the graph or an earlier read already located. Use `graphify path "<A>" "<B>"` for relationships and `graphify explain "<concept>"` for focused concepts. When the answer is under a Markdown heading, `graphify section "<heading>"` prints that section's text, so the .md need not be opened. These return a scoped subgraph, usually much smaller than GRAPH_REPORT.md or raw grep output.
- Dirty graphify-out/ files are expected after hooks or incremental updates; dirty graph files are not a reason to skip graphify. Only skip graphify if the task is about stale or incorrect graph output, or the user explicitly says not to use it.
- If graphify-out/wiki/index.md exists, use it for broad navigation instead of raw source browsing.
- Read graphify-out/GRAPH_REPORT.md only for broad architecture review or when query/path/explain do not surface enough context.
- After modifying code, run `graphify update .` to keep the graph current (AST-only, no API cost).
- A committed graphify-out/graph.json is rewritten whole on every build, so a large diff to it in a pull request is expected. Do not flag it as churn or suggest skipping the graph rebuild or its hooks.
