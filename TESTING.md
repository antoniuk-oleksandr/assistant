# Testing Strategy

## 1. Goals

Tests must prove observable behavior of the application and protect the boundaries described in `ARCHITECTURE.md`.

The test suite should answer:

- does the use case behave correctly;
- are invalid states rejected;
- are external failures translated and retried correctly;
- are SQLite state transitions durable and idempotent;
- do mappers prevent model leakage;
- does the application recover after restart;
- does the CLI call the correct application service?

## 2. Test layers

### Unit tests

Unit tests cover Domain and Application behavior without real external systems.

Use:

- Ginkgo and Gomega for specifications;
- `uber-go/mock` for ports and external dependencies;
- deterministic fixtures from `tests/support/fixtures`;
- a controlled `Clock` for time-dependent behavior.

Unit tests must not call Google Classroom, Google Drive, SMTP, Telegram, or production files.

### Persistence integration tests

Persistence tests use an isolated temporary SQLite database. Each test or suite must have deterministic state and must not use the application's real database.

They verify:

- migrations;
- repository CRUD behavior;
- unique constraints;
- status transitions;
- task-to-scenario bindings;
- checkpoint recovery;
- notification retry and DLQ persistence;
- monthly cleanup rules.

### External integration tests

Tests for Google Classroom, Drive, Docs, Sheets, email, and Telegram are opt-in integration tests. They require explicit credentials and are never part of the default unit-test command.

The default suite uses mocks for these ports.

## 3. Test organization

```text
mocks/
├── submission_source_mock.go
├── document_parser_mock.go
├── notification_provider_mock.go
└── repositories_mock.go

tests/
├── unit/
│   ├── domain/
│   ├── application/
│   ├── presentation/
│   └── infrastructure/
├── integration/
│   └── sqlite/
├── support/
│   └── fixtures/
│       ├── tasks.go
│       ├── submissions.go
│       ├── materials.go
│       ├── reports.go
│       ├── teachers.go
│       └── notifications.go
└── testdata/
    ├── docx/
    ├── pdf/
    └── images/
```

## 4. Fixtures

Fixtures are shared builders with valid defaults and focused overrides. Tests must not repeat large object construction inline.

```go
task := fixtures.Task(
    fixtures.WithTaskID("task-1"),
    fixtures.WithDueAt(dueAt),
)

submission := fixtures.SubmittedSubmission(
    fixtures.ForTask(task.ID),
    fixtures.WithMaterial(fixtures.DocxMaterial()),
)
```

Required fixture states include:

- active and disabled scenarios;
- bound and unbound tasks;
- submitted and non-submitted submissions;
- missing, invalid, and access-denied materials;
- valid and conflicting records;
- active and disabled teachers/channels;
- pending, sent, retrying, and dead-letter notifications;
- every important `ReportRun` status.

Fixtures create data. Mocks define external interactions. The test asserts behavior.

## 5. Mocks

Mocks are generated from interfaces and stored in the top-level `mocks/` directory.

```go
//go:generate mockgen -source=submission_source.go -destination=../../mocks/submission_source_mock.go -package=mocks
```

Generation and execution are separate:

```text
go generate ./...
go test ./...
```

`go test` does not execute `go:generate` directives.

## 6. Required behavior coverage

### Scenario and task management

- `ScenarioNotFound` when implementation is not registered;
- adding, enabling, disabling, and removing scenarios;
- binding and unbinding tasks;
- refusing to process unbound tasks;
- selecting the correct scenario for a task.

### Polling and recovery

- five-minute polling interval;
- `last_check_timestamp` recovery;
- no duplicate process;
- no duplicate task discovery;
- idempotent repeated polling;
- restart during an active `ReportRun`.

### Submissions and parsing

- submitted versus non-submitted state;
- missing attachment;
- inaccessible attachment;
- parser selection by material type;
- invalid document;
- mapper conversion errors;
- preservation of source metadata.

### Aggregation

- numeric values and dashes as zero;
- text merging;
- duplicate handling;
- conflicting values;
- source traceability;
- partial and completed results.

### Notifications

- reminder timing three days before the deadline;
- final notification after the deadline;
- disabled teacher/channel;
- retry delays of 5, 10, and 30 seconds;
- movement to DLQ after three failures;
- manual retry of one DLQ record;
- retry-all behavior;
- duplicate notification prevention.

### Presentation

- Cobra argument validation;
- correct service invocation;
- human-readable success output;
- stable error output and exit status;
- no direct database or provider access from commands.

## 7. Commands

Before considering a change complete:

```text
go generate ./...
gofmt -w .
go vet ./...
go test -race ./...
```

If a command cannot run in the current environment, report it as blocked with the exact reason. Do not claim the test passed.

## 8. Test naming

Tests describe behavior, not implementation details:

```text
it stores a submitted Classroom submission
it moves a notification to the DLQ after three failed attempts
it rejects a task that is not bound to a scenario
it maps a Google submission into a domain submission
```

Avoid tests that assert private helper calls, SQL statement text, or internal call order unless that order is part of the observable contract.

## 9. Test boundaries

- Never use production credentials or production data.
- Never send real email or Telegram messages in default tests.
- Never weaken or delete a failing test to make a production implementation pass.
- The Tester may change tests and fixtures, but not production behavior.
- Tests must remain deterministic and independent.
