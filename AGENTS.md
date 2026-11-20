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
