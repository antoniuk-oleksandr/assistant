# Application Architecture

## 1. Technology choices

The application is implemented in Go and is distributed as a single executable.

Core libraries:

- `go.uber.org/fx` — dependency injection and application lifecycle;
- `go.uber.org/zap` — structured logging;
- `go.uber.org/mock` — generated mocks for interfaces;
- `github.com/spf13/cobra` — CLI commands, arguments, flags, help, and completion;
- Ginkgo — test structure and test runner;
- Gomega — assertions and matchers;
- SQLite driver — local persistence;
- Google API clients — Google Classroom, Drive, Docs, and Sheets integration;
- standard library `context` — cancellation, deadlines, and lifecycle propagation.

The application follows four layers:

```text
Domain
    ↑
Application
    ↑
Infrastructure
    ↑
Presentation
```

The dependency direction is inward: Domain contains business concepts, Application coordinates use cases, Infrastructure implements technical integrations, and Presentation exposes the application to users. The application must keep domain logic independent from Google APIs, SQLite, CLI parsing, notification providers, and concrete document parsers.

## 2. Executable modes

The product has one executable and one entry point. It supports two runtime modes:

```text
report-system.exe
    ├── no arguments → background application
    └── command arguments → CLI command
```

The background application starts the five-minute polling loop by default. CLI commands use the same application modules and repositories.

Recommended commands:

```text
report-system.exe

report-system.exe scenario list
report-system.exe scenario add science-report
report-system.exe scenario enable science-report
report-system.exe scenario disable science-report
report-system.exe scenario remove science-report
report-system.exe course list
report-system.exe course add <course-url-or-id>
report-system.exe course remove <course-id>

report-system.exe teacher list
report-system.exe teacher add --email user@example.com --name "User Name" --classroom <course-id>

report-system.exe notification retry <id>
report-system.exe notification dlq list
report-system.exe notification dlq retry-all
```

The default invocation starts the long-running application. CLI commands perform one administrative operation and exit.

`course add <reference>` accepts a Google Classroom URL, a numeric course ID, or the
Base64URL segment from a Classroom URL. The command normalizes and validates the
ID before persisting it. `course list` displays registered courses.
`course remove <course-id>` disables polling for the course while preserving
its historical tasks, submissions, report runs, and audit records.

## 3. Source tree

```text
cmd/
└── report-system/
    ├── main.go
    └── commands/
        ├── root.go
        ├── scenario.go
        ├── teacher.go
        └── notification.go

internal/
├── app/
│   ├── background.go
│   └── cli.go
├── domain/
│   ├── scenario/
│   ├── task/
│   ├── submission/
│   ├── report/
│   ├── notification/
│   ├── teacher/
│   └── ports/
├── application/
│   ├── scenario_service.go
│   ├── polling_service.go
│   ├── report_service.go
│   └── notification_service.go
├── presentation/
│   ├── dto/
│   ├── mappers/
│   └── cobra/
├── adapters/
│   ├── classroom/
│   ├── drive/
│   ├── docs/
│   ├── sheets/
│   ├── parser/
│   ├── renderer/
│   ├── notification/
│   ├── storage/
│   ├── models/
│   └── mappers/
├── templates/
│   └── scientific-report/
│       ├── appendix-01.docx
│       ├── appendix-02.docx
│       └── appendix-28.docx
├── platform/
│   ├── config/
│   ├── database/
│   ├── logging/
│   ├── locking/
│   ├── scheduling/
│   └── time/
└── fxmodules/
    ├── platform.go
    ├── storage.go
    ├── classroom.go
    ├── parsing.go
    ├── notifications.go
    └── application.go

mocks/
├── submission_source_mock.go
├── document_parser_mock.go
├── notification_provider_mock.go
└── repositories_mock.go

tests/
├── unit/
├── integration/
└── support/
    └── fixtures/
        ├── tasks.go
        ├── submissions.go
        ├── materials.go
        ├── reports.go
        ├── teachers.go
        └── notifications.go
```

The exact package layout may change, but dependency direction must remain stable:

```text
Presentation → Application → Domain
       │            │          ↑
       └────────────┴── Infrastructure
```

Domain must not import any outer layer. Application may depend on Domain, including Domain ports. Infrastructure implements Domain ports. Presentation calls Application services and does not access Infrastructure directly.

## 3.1. Cobra CLI

The CLI uses Cobra with a root command and nested subcommands. Cobra commands are transport adapters: they parse arguments and flags, call application services, print results, and return errors. They must not contain SQL, Google API calls, or business rules.

Recommended command tree:

```text
report-system
├── course
│   ├── list
│   ├── add <course-url-or-id>
│   ├── tasks <course-url-or-id>
│   └── remove <course-id>
├── scenario
│   ├── list
│   ├── add <name>
│   ├── enable <name>
│   ├── disable <name>
│   ├── remove <name>
│   ├── course-bind <scenario> <course-id>
│   ├── course-unbind <scenario> <course-id>
│   ├── course-list <scenario>
│   ├── data-set <scenario> <course-id> --field name=value
│   ├── data-list <scenario> <course-id>
│   ├── task-bind <scenario> <task-id>
│   ├── task-unbind <task-id>
│   └── task-list <scenario>
├── teacher
│   ├── list
│   ├── show <id>
│   ├── add
│   ├── edit <id>
│   ├── enable <id>
│   ├── disable <id>
│   ├── remove <id>
│   └── channel
└── notification
    ├── retry <id>
    └── dlq
        ├── list
        └── retry-all
```

`scenario data-set <scenario> <course-id> --field name=value` is a persistence
command for course-specific report metadata. It updates only the named fields
in SQLite; report rendering remains part of the polling workflow. The command
must reject an inactive or unbound scenario-course pair.

Each command should use `RunE` so errors are returned to the root executor rather than handled inside command logic.

```go
func NewScenarioAddCommand(service ScenarioService) *cobra.Command {
    var command = &cobra.Command{
        Use:   "add <scenario-name>",
        Short: "Register a scenario",
        Args:  cobra.ExactArgs(1),
        RunE: func(cmd *cobra.Command, args []string) error {
            return service.Add(cmd.Context(), args[0])
        },
    }
    return command
}
```

Command constructors receive application interfaces through Fx. A command must not call `fx.New`, create a repository, or read environment variables.

The CLI may provide shell completion and generated help through Cobra. The Cobra project recommends a small `main.go`, a root command, and separate files/packages for subcommands. [Cobra user guide](https://github.com/spf13/cobra/blob/main/site/content/user_guide.md)

## 3.2. Internal configuration

The application has a typed internal configuration object loaded once during Fx startup. Environment variables and the local `.env` file are input sources; application code receives typed configuration through dependency injection.

```go
type Config struct {
    Database DatabaseConfig
    Google   GoogleConfig
    Email    EmailConfig
    Telegram TelegramConfig
    Logging  LoggingConfig
    Runtime  RuntimeConfig
}

type RuntimeConfig struct {
    PollInterval        time.Duration
    CleanupInterval     time.Duration
    NotificationRetries []time.Duration
    LogDirectory        string
}
```

The loader validates required values and returns an error before the application starts if a required setting is missing or invalid.

```go
func Load() (Config, error) {
    // Read environment variables and .env values.
    // Parse durations, paths, URLs, and numeric values.
    // Validate required settings.
    // Return a typed immutable configuration value.
}
```

The configuration contains technical runtime settings only:

```text
Config
├── Database
│   └── SQLitePath
├── Google
│   ├── CredentialsPath
│   ├── CourseID
│   └── OAuthTokenPath
├── Email
│   ├── SMTPHost
│   ├── SMTPPort
│   ├── Username
│   └── Password
├── Telegram
│   └── BotToken
├── Logging
│   ├── Directory
│   └── Level
└── Runtime
    ├── PollInterval
    ├── CleanupInterval
    └── NotificationRetries
```

Scenario registration, task-to-scenario bindings, teachers, notification channels, checkpoints, report runs, and delivery states are not configuration. They are runtime data and must be stored in SQLite.

There is no scenario YAML/JSON configuration. Adding or enabling a scenario is done through the CLI and persisted in the database.

Fx provides the configuration to constructors:

```go
var PlatformModule = fx.Module(
    "platform",
    fx.Provide(
        config.Load,
        database.NewSQLite,
        logging.New,
        scheduling.New,
    ),
)
```

Constructors must receive only the configuration section they need rather than reading environment variables directly.

```go
func NewSQLite(cfg config.DatabaseConfig) (*sql.DB, error)
func NewGoogleClient(cfg config.GoogleConfig) (GoogleClient, error)
func NewEmailProvider(cfg config.EmailConfig, logger *zap.Logger) NotificationProvider
```

This keeps environment access at the application boundary and makes services deterministic and easy to test.

## 4. Clean Fx bootstrap

The executable `main` must only assemble and run the Fx application. It must not contain business logic or manually construct repositories, clients, loggers, or services.

The single executable also includes the Cobra module. There is no separate CLI executable and no second `main.go`.

```go
func main() {
    app := fx.New(
        fxmodules.PlatformModule,
        fxmodules.StorageModule,
        fxmodules.ClassroomModule,
        fxmodules.ParsingModule,
        fxmodules.NotificationModule,
        fxmodules.ApplicationModule,
        fxmodules.CLIModule,
        fx.Invoke(fxmodules.StartMode),
    )

    ctx := context.Background()

    if err := app.Start(ctx); err != nil {
        log.Fatal(err)
    }

    defer func() {
        if err := app.Stop(ctx); err != nil {
            log.Print(err)
        }
    }()

    <-app.Done()
}
```

`CLIModule` provides the Cobra root command and its subcommands through Fx:

```go
var CLIModule = fx.Module(
    "cli",
    fx.Provide(
        NewRootCommand,
        NewScenarioCommand,
        NewTeacherCommand,
        NewNotificationCommand,
    ),
)
```

`StartMode` receives the process arguments and selects the runtime mode:

```text
no arguments
    → start the background polling service

arguments present
    → execute the Cobra root command
    → run one administrative operation
    → stop the Fx application
```

The root command and subcommands call application services. They do not create repositories, Google clients, notification providers, or database connections themselves.

`StartMode` receives `os.Args` through Fx and chooses the mode. The CLI must not construct SQLite repositories or notification providers itself; it receives application services through Fx.

## 5. Fx modules

Each module owns constructors for one technical area.

```go
var PlatformModule = fx.Module(
    "platform",
    fx.Provide(
        config.Load,
        logging.New,
        database.NewSQLite,
        locking.NewProcessLock,
        timefx.NewClock,
    ),
)

var StorageModule = fx.Module(
    "storage",
    fx.Provide(
        storage.NewScenarioRepository,
        storage.NewTaskRepository,
        storage.NewSubmissionRepository,
        storage.NewReportRunRepository,
        storage.NewTeacherRepository,
        storage.NewNotificationRepository,
    ),
)

var ClassroomModule = fx.Module(
    "classroom",
    fx.Provide(
        classroom.NewOAuthClient,
        classroom.NewSubmissionSource,
    ),
)
```

Lifecycle hooks belong in constructors or dedicated `fx.Invoke` functions:

- open and close the database;
- acquire and release the process lock;
- start and stop the polling scheduler;
- flush and sync the logger;
- close HTTP clients and provider resources.

## 6. Interfaces and dependency direction

Ports belong to the Domain layer and define the contracts required by the business and application workflows. Infrastructure adapters implement those ports.

```go
type ScenarioRepository interface {
    Create(ctx context.Context, scenario Scenario) error
    GetByName(ctx context.Context, name string) (Scenario, error)
    ListActive(ctx context.Context) ([]Scenario, error)
    Enable(ctx context.Context, name string) error
    Disable(ctx context.Context, name string) error
    Remove(ctx context.Context, name string) error
}
```

The SQLite implementation is a separate Infrastructure module:

```go
type SQLiteScenarioRepository struct {
    db    *sql.DB
    clock Clock
}

func NewScenarioRepository(db *sql.DB, clock Clock) ScenarioRepository {
    return &SQLiteScenarioRepository{db: db, clock: clock}
}
```

The constructor returns the interface so callers depend on behavior, not implementation.

## 6.1. Models, DTOs, and mappers

Models must not leak across layer boundaries.

```text
Infrastructure model
    ↓ Infrastructure mapper
Domain model
    ↓ Application use case
Application result
    ↓ Presentation mapper
Presentation DTO
```

Each layer owns its data structures:

- **Infrastructure models** represent Google API responses, SQLite rows, filesystem metadata, and provider payloads;
- **Domain models** represent business entities, value objects, and rules;
- **Application models** represent use-case commands, results, and errors;
- **Presentation DTOs** represent Cobra input, output, flags, and user-facing error data.

Examples of forbidden leaks:

- a Google `StudentSubmission` type must not appear in Domain or Application;
- a `sql.NullString` or SQLite row model must not appear in Domain;
- a Domain entity must not be printed directly by Cobra;
- a Presentation DTO must not be accepted by a Domain service.

Mappers are explicit modules with public APIs:

```go
type ClassroomSubmissionMapper interface {
    ToDomain(source ClassroomSubmissionModel) (Submission, error)
}

type SubmissionResponseMapper interface {
    ToDTO(submission Submission) SubmissionDTO
}
```

Mapping responsibilities:

```text
adapters/classroom/mappers
    Google Classroom API model → Domain Submission

adapters/storage/mappers
    SQLite row model → Domain entity
    Domain entity → SQLite row model

presentation/mappers
    Application result → Presentation DTO
    Cobra flags/arguments → Application command
```

Mapping belongs at the boundary where the foreign model enters or leaves a layer. Mappers must not contain business workflows; they only translate structure, types, identifiers, and validation errors.

Other important ports:

```go
type SubmissionSource interface {
    ListTasks(ctx context.Context, courseID string) ([]Task, error)
    ListSubmissions(ctx context.Context, taskID string) ([]Submission, error)
}

type DocumentParser interface {
    CanHandle(material Material) bool
    Parse(ctx context.Context, material Material) (ParsedDocument, error)
}

type MaterialAnalyzer interface {
    Analyze(ctx context.Context, material Material) (ExtractionProposal, error)
}

type ReportRenderer interface {
    Render(ctx context.Context, report Report) (ReportPackage, error)
    Save(ctx context.Context, package ReportPackage, dir string) (SavedReportPackage, error)
}

type NotificationProvider interface {
    Name() string
    Validate(ctx context.Context, recipient Recipient) error
    Send(ctx context.Context, message Message, recipient Recipient) (DeliveryResult, error)
}

type Clock interface {
    Now() time.Time
}
```

Do not create interfaces for every struct automatically. Create an interface when a dependency needs substitution, has multiple implementations, or must be isolated in a unit test.

## 7. Application services

Application services coordinate use cases and transactions. They do not know SQL details or Google HTTP request details.

```text
ScenarioService
├── add
├── enable
├── disable
├── remove
├── bind course
└── bind task within course

PollingService
├── acquire process lock
├── read last checkpoint
├── discover tasks
├── refresh submissions
├── create/update ReportRun
└── persist checkpoint

ReportService
├── parse materials
├── validate data
├── aggregate records
├── detect conflicts
└── render and save local report package

NotificationService
├── resolve active channels
├── create delivery records
├── send notifications
├── schedule retries
└── move exhausted deliveries to DLQ
```

## 8. Architecture and design patterns

Only patterns with a concrete role in this application should be introduced. The architectural style and the individual object-level patterns are separate concepts.

### 8.1. Architecture: four layers with Ports and Adapters

The application is organized into four layers:

```text
Domain
├── entities
├── value objects
├── domain rules
└── domain errors

Application
├── use cases
├── application services
└── transaction boundaries

Domain ports
├── submission source
├── repositories
├── parsers
├── renderers
├── notification providers
└── clock

Infrastructure
├── Google Classroom/Drive/Docs/Sheets adapters
├── SQLite repositories
├── document parsers
├── report renderers
├── email and Telegram providers
└── logging, locking, and scheduling

Presentation
├── Cobra root command
├── scenario commands
├── teacher commands
├── notification commands
└── console output and exit codes
```

Ports and Adapters describes how Domain contracts are implemented by Infrastructure:

```text
Domain port
    ↓
Port / Interface
    ↑
Adapter
    ↓
Infrastructure adapter
```

Domain defines ports. Application services use those ports, while Infrastructure integrations implement them. The Domain and Application layers do not depend on Google Classroom, SQLite, SMTP, Telegram, or a particular parser library.

For example:

```text
Application Service
        ↓
SubmissionSource port
        ↑
GoogleClassroomAdapter
        ↓
Google Classroom API
```

`Ports and Adapters` is an architectural style, not the GoF Adapter pattern. Presentation is a separate outer layer; it is not an external system.

### 8.2. Infrastructure and integration patterns

#### Dependency injection

Fx owns construction and lifecycle of production dependencies. Application code receives interfaces and does not construct infrastructure services itself.

#### Adapter

The GoF Adapter is a concrete object that translates an external API or data model into one of the application ports:

- `GoogleClassroomAdapter` implements `SubmissionSource`;
- `SQLiteScenarioRepository` implements `ScenarioRepository`;
- `SMTPNotificationAdapter` implements `NotificationProvider`;
- `TelegramNotificationAdapter` implements `NotificationProvider`.

The adapter pattern is one implementation mechanism inside the broader Ports and Adapters architecture.

#### Repository

Repositories encapsulate SQLite persistence for scenarios, task bindings, submissions, report runs, teachers, channels, notifications, conflicts, and checkpoints. Repositories are persistence patterns, not GoF Adapter implementations, even though a SQLite repository also adapts database operations to an application interface.

#### Mapper

Mapper is a boundary pattern used to translate infrastructure models, domain models, application results, and presentation DTOs. It prevents external schemas from leaking into another layer. Mappers are explicit and focused; they do not contain use-case orchestration.

#### Application Service / Facade

Application services act as facades for complete use cases. For example, `ReportService` coordinates parsing, validation, aggregation, conflict detection, and rendering without exposing those steps to the CLI or scheduler.

#### Decorator

Decorators wrap ports with cross-cutting behavior:

```text
NotificationProvider
    ↓
RetryingNotificationProvider
    ↓
LoggingNotificationProvider
    ↓
SMTPNotificationAdapter
```

The same approach can wrap Google clients with logging, metrics, or retry behavior without changing the underlying adapter.

### 8.3. Object-level patterns

#### Factory

Factories select implementations at runtime:

- `ParserFactory` selects a document parser by material type;
- `NotificationProviderFactory` selects email or Telegram;
- `RendererFactory` selects the report output format;
- `ScenarioRegistry` resolves a registered scenario implementation.

Factories are small and explicit. Business workflows remain in application services.

#### Strategy

Strategies represent interchangeable behavior:

- DOCX, PDF, Google Docs, and Google Sheets parsers for the 28 scientific-report appendices;
- text and table aggregation strategies;
- report renderers;
- notification providers.

The caller uses the interface and does not know which implementation was selected.

#### Persisted statuses

`ReportRun`, notification delivery, and dead-letter records have explicit status fields stored in SQLite. These are persistence and workflow rules, not separate GoF State objects. The application validates allowed status transitions and makes them idempotent.

Cobra commands are part of the CLI adapter layer. They parse input and invoke application services; they are not modeled as GoF Command objects.

#### Builder

Builders are used for test fixtures only. They make tests concise while keeping production domain constructors explicit.

## 9. Logging with zap

All application logging uses structured `zap.Logger`.

```go
logger.Info("submission received",
    zap.String("task_id", taskID),
    zap.String("teacher_email", teacherEmail),
    zap.String("state", state),
)
```

Logs are written to daily local files under `logs/`. Secrets, OAuth tokens, passwords, and full document contents must never be logged.

Recommended fields:

- `scenario_name`;
- `task_id`;
- `report_run_id`;
- `submission_id`;
- `teacher_email` where appropriate;
- `provider`;
- `attempt`;
- `error`.

## 10. Testing

The project uses unit tests with Ginkgo and Gomega. External dependencies are replaced through interfaces and mocks generated by `uber-go/mock`.

```go
type SubmissionSource interface {
    ListSubmissions(ctx context.Context, taskID string) ([]Submission, error)
}

//go:generate mockgen -source=submission_source.go -destination=../../mocks/submission_source_mock.go -package=mocks
```

Generated mocks live in the top-level `mocks/` directory and are not production implementations. Tests import them as a separate package.

Example test shape:

```go
var _ = Describe("PollingService", func() {
    It("stores a submitted Classroom submission", func() {
        source := mocks.NewMockSubmissionSource(ctrl)
        source.EXPECT().ListSubmissions(gomock.Any(), "task-1").Return(
            []Submission{{State: Submitted}}, nil,
        )

        err := service.Refresh(ctx, "task-1")
        Expect(err).NotTo(HaveOccurred())
    })
})
```

Unit tests must cover:

- `ScenarioNotFound`;
- task binding and disabled scenarios;
- restart from `last_check_timestamp`;
- duplicate process prevention;
- submitted versus non-submitted state;
- parser selection;
- missing or inaccessible attachments;
- aggregation and conflicts;
- notification retries at 5, 10, and 30 seconds;
- DLQ movement and manual retry commands;
- monthly cleanup rules.

The application uses interfaces and `uber-go/mock` for substituting external dependencies in tests.

## 10.1. Test fixtures

Tests must use shared fixtures instead of repeatedly constructing domain objects inline. Fixtures are reusable builders or factory functions located in `tests/support/fixtures`.

Example fixture API:

```go
func Task(overrides ...TaskOption) Task
func SubmittedSubmission(overrides ...SubmissionOption) Submission
func PendingSubmission(overrides ...SubmissionOption) Submission
func DocxMaterial(overrides ...MaterialOption) Material
func Teacher(overrides ...TeacherOption) Teacher
func ReportRun(overrides ...ReportRunOption) ReportRun
```

Example usage:

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

Fixtures should provide valid defaults and allow focused overrides. A test should describe only the values relevant to the behavior under test.

Required shared fixture categories:

- tasks bound and unbound to scenarios;
- submitted and non-submitted submissions;
- accessible, missing, and access-denied materials;
- valid and invalid parsed documents;
- report runs in each important state;
- active and disabled teachers;
- active and disabled notification channels;
- successful, retrying, and dead-letter deliveries;
- open and resolved data conflicts.

Fixtures must not hide the behavior being tested. They create data; mocks define external interactions; the test asserts the resulting behavior.

Mock generation and test execution are separate steps:

```text
go generate ./...
go test ./...
```

`go test` does not execute `go:generate` directives. CI should run mock generation before tests, and the generated files should either be committed to the repository or be generated in every clean CI build.

## 11. Error model

Errors should be typed or wrapped with stable categories:

```text
ScenarioNotFound
ApplicationAlreadyRunning
AuthenticationFailed
PermissionDenied
ExternalResourceNotFound
AttachmentAccessDenied
InvalidDocument
ParseFailed
DataConflict
NotificationDeliveryFailed
DatabaseFailure
```

Application services decide whether an error is retryable. Adapters return provider-specific details; domain code works with stable application error categories.

## 12. Implementation principles

- Keep `main.go` free of construction logic and business logic.
- Keep business rules in application/domain packages.
- Keep Google API details in adapters.
- Keep SQLite queries in repository implementations.
- Keep parser selection in a registry.
- Keep scenario state in SQLite.
- Keep notification state in SQLite.
- Make state transitions idempotent.
- Pass `context.Context` through every I/O operation.
- Use interfaces at integration boundaries.
- Prefer simple explicit code over speculative generalization.

## 13. Engineering rules

The codebase follows SOLID, DRY, and KISS as mandatory engineering principles.

### SOLID

- **Single Responsibility** — each package, type, and function has one clear responsibility.
- **Open/Closed** — new parsers, providers, renderers, and scenarios are added through interfaces and registration rather than by rewriting unrelated services.
- **Liskov Substitution** — every implementation must honor the behavior promised by its interface.
- **Interface Segregation** — interfaces contain only the operations required by their consumer.
- **Dependency Inversion** — application services depend on ports; infrastructure depends on those ports.

### DRY

Shared behavior is implemented once and reused through a package API, fixture, service, or interface. Copy-pasted parsing, retry, validation, persistence, and notification logic is not allowed.

### KISS

Implement the smallest clear solution that satisfies the scenario. Avoid speculative abstractions, hidden magic, unnecessary reflection, and framework layers that do not support a current use case.

### Small functions

Functions should perform one understandable operation. A function that coordinates several independent operations should delegate them to application services or focused package APIs rather than becoming a large procedural block.

### Small modules

Packages are small and cohesive. A package should expose one clear capability, for example:

```text
internal/parser/docx
internal/notification/email
internal/report/render
internal/retry
internal/teacher
```

### No hidden private helpers

Reusable or meaningful helper logic must not be buried in private functions inside an unrelated package. If auxiliary behavior is needed, create a small module with an explicit public API:

```go
package retry

type Policy struct {
    Delays []time.Duration
}

func NewPolicy(delays []time.Duration) Policy
func Execute(ctx context.Context, policy Policy, operation Operation) error
```

The rule is to make module boundaries and reusable behavior visible. Business behavior and reusable helpers must be exposed through a deliberate package API rather than hidden in ad-hoc private functions, types, or methods.

### Explicit APIs

Every mini-module should have:

- a clear package name;
- exported input and output types where they are part of the contract;
- a constructor or factory when dependencies are required;
- interfaces at external boundaries;
- focused unit tests and reusable fixtures.
