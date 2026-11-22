# Scheduled Report Aggregation System

[![codecov](https://codecov.io/gh/antoniuk-oleksandr/assistant/graph/badge.svg?token=KM23EQMH96)](https://codecov.io/gh/antoniuk-oleksandr/assistant)

Scheduled Report Aggregation System is a local Go application for collecting, validating, aggregating, and distributing periodic departmental submissions. Its first concrete workflow is scientific activity reporting from Google Classroom.

It discovers Classroom coursework, reads submissions and attached materials, maps them to a fixed 28-appendix scientific-report structure, preserves supporting evidence, creates an aggregate report, and stores operational state in SQLite.

## Runtime model

There is one executable and no separate `worker` command:

```text
no arguments  -> start the long-running background poller
arguments     -> execute one administrative CLI command and exit
```

The background process waits for the next persisted report deadline event. It
uses a SQLite process lock, so only one instance can run against a database.

```bash
just run
# equivalent:
go run ./cmd/assistent
```

The command normally stays attached to the terminal and produces little output. Stop it with `Ctrl+C`. A second process using the same database exits because the lock is held.

## Features

- Google Classroom course registration and polling.
- One report workflow configured per registered Classroom course.
- Scientific activity reporting through the built-in report processor.
- Mapping to 28 known appendices with defaults for missing appendices.
- DOCX, XLSX, PDF, Google Docs, Google Sheets, and image ingestion boundaries.
- Supporting evidence and source-reference preservation.
- Numeric aggregation, text aggregation, and conflict detection.
- Persisted report runs, versioned results, outputs, and processing history.
- OAuth-backed Google Sheets output as the production default.
- Local XLSX output as an explicit fallback.
- Missing-submission reminders.
- Notification retries and dead-letter state.
- Teacher recipients and notification channels in SQLite.
- Restart checkpoints and idempotent polling state.
- Local daily application logs.

## Requirements

- Go 1.20 or newer.
- SQLite support through `github.com/mattn/go-sqlite3`.
- Google Cloud OAuth credentials for live Classroom, Drive, and Sheets access.
- A Google OAuth **Web application** client with
  `https://developers.google.com/oauthplayground` as an authorized redirect URI
  when generating the refresh token through OAuth Playground.
- The Google account used for authorization must be added under the OAuth
  consent screen's **Test users** while the app is in Testing mode.
- `just` is optional.

The application uses one shared OAuth client and refresh token for Google
Classroom, Drive, and Sheets. The authorized account must be able to access the
registered Classroom course and its submissions.

Required Google Cloud APIs:

- Google Classroom API
- Google Drive API
- Google Sheets API

Required OAuth scopes:

```text
https://www.googleapis.com/auth/classroom.coursework.students.readonly
https://www.googleapis.com/auth/classroom.rosters.readonly
https://www.googleapis.com/auth/classroom.profile.emails
https://www.googleapis.com/auth/drive.readonly
https://www.googleapis.com/auth/drive.file
https://www.googleapis.com/auth/spreadsheets
```

## Configuration

Create a repository-root `.env` file. Never commit it.

### Database and renderer

```env
# Default: data/report-system.db
SQLITE_DATABASE_PATH=data/report-system.db

# Default: google-sheets. Alternative: local-xlsx
REPORT_RENDERER=google-sheets

GOOGLE_SHEETS_REPORT_TITLE=Scientific activity report
GOOGLE_SHEETS_BASE_URL=https://sheets.googleapis.com/v4

# Used to interpret Google Classroom due dates and times.
GOOGLE_CLASSROOM_TIMEZONE=Europe/Kyiv

# Deadline windows; supports 3d, 5m, 2h, and other positive durations.
REPORT_PRELIMINARY_BEFORE=3d
REPORT_REFRESH_BEFORE=1d

```

For fast local testing, set `REPORT_PRELIMINARY_BEFORE=2m` and
`REPORT_REFRESH_BEFORE=1m`. The scheduler waits for those exact deadline offsets.

### Google OAuth

```env
GOOGLE_CLIENT_ID=...
GOOGLE_CLIENT_SECRET=...
GOOGLE_REFRESH_TOKEN=...
```

The three values identify one OAuth client and one authorized Google account. The
refresh token is a secret used to obtain short-lived access tokens without
interactive authorization. The Google Cloud project must enable the Google
Classroom, Google Drive, and Google Sheets APIs and configure the OAuth consent
screen for the scopes above. When OAuth Playground is used, the OAuth client
must be a Web application and must authorize this exact redirect URI:

```text
https://developers.google.com/oauthplayground
```

If the OAuth consent screen is in Testing mode, the account that authorizes the
application must also be listed as an approved test user.

### Report-ready email

When Google SMTP is configured, the application sends a report-ready email to each
active Classroom participant with an email address after creating a Google Sheet. The message body is read from
`templates/report-ready-email.txt` by default, and the
`<google_sheet_link>` tag is replaced with the generated Sheet URL.

After the email is sent, the same rendered message is sent to the configured
Telegram chat when an active recipient has a paired Telegram channel configured.

Enable 2-Step Verification for the Google account and create a Google app
password. Google SMTP uses STARTTLS on port 587.

```env
SMTP_HOST=smtp.gmail.com
SMTP_PORT=587
SMTP_USERNAME=sender@gmail.com
SMTP_PASSWORD=your-google-app-password
SMTP_FROM=sender@gmail.com
# Optional custom template:
# REPORT_READY_EMAIL_TEMPLATE=templates/report-ready-email.txt
# Optional missing-submission reminder template:
# REMINDER_TEMPLATE_PATH=templates/missing-submission-reminder.txt
```

Report recipients are not configured in `.env`. Google Classroom participants
are synchronized automatically, including their email addresses. Active
participants with an email address receive the generated report.

Google Classroom participants are synchronized into the local teacher directory
during polling. Generate a private Telegram binding link for an automatically
discovered participant:

```bash
go run ./cmd/assistent teacher list <course-id-or-url>

go run ./cmd/assistent teacher telegram-bind <teacher-id-or-classroom-user-id>
```

The command prints a one-time Telegram pairing link. The teacher opens that
link and presses Start; the app then verifies the token and stores the user's
private Telegram chat ID for future notifications. No phone number is required.

During the configured preliminary-before-deadline phase, the application creates one
email reminder per missing roster participant. The reminder includes the task
title, due date, remaining time, and Classroom link. Reminder attempts and
failures are persisted in SQLite and deduplicated by task, participant, channel,
and phase. The default body is `templates/missing-submission-reminder.txt`;
override it with `REMINDER_TEMPLATE_PATH` and use `<task_title>`, `<due_at>`,
`<remaining_time>`, and `<classroom_link>` placeholders.

At the same configured preliminary deadline, the administrator receives one
summary by email and Telegram. It lists users who have not sent their files;
when none are missing, it says that everybody has already sent the files. The
active Telegram binding identifies the administrator's private chat.
`TELEGRAM_CHAT_ID` remains a legacy fallback when no paired chat exists.

### Google Drive settings

```env
# Default: data/materials
GOOGLE_DRIVE_MATERIAL_DIR=data/materials
GOOGLE_DRIVE_BASE_URL=https://www.googleapis.com/drive/v3
```

See [docs/drive.md](docs/drive.md) for Drive authorization details and
[docs/sheets.md](docs/sheets.md) for Google Sheets API requirements.

The OAuth client, consent configuration, enabled APIs, and refresh token belong
to the same Google Cloud project and authorized account. The account's
Classroom course and user identifiers are discovered through the Classroom API;
report Sheet identifiers are generated by the application.

## CLI commands

The executable built from this repository is `assistent`. Use `--help` for Cobra help:

```bash
go run ./cmd/assistent --help
```

### Report workflow

```bash
go run ./cmd/assistent report configure <course-id>
go run ./cmd/assistent report task-list
go run ./cmd/assistent report task-bind <task-id>
go run ./cmd/assistent report task-unbind <task-id>

# Store course-specific report fields (repeat --field for multiple values)
go run ./cmd/assistent report data-set <course-id> \
  --field department_email=department@example.com \
  --field department_name="Department name" \
  --field lab_assistant_name="Anna Kovalenko" \
  --field lab_assistant_mobile_phone="+380..." \
  --field department_phone="+380..."
go run ./cmd/assistent report data-list <course-id>
```

`report configure` enables the built-in report workflow for a registered
course. `report data-set` stores course-specific report values in SQLite; it
does not create a Google Sheet by itself. The background poller uses the saved
values when it generates the next report. Each value is supplied as a separate
`--field name=value` flag.

A task must be explicitly bound with `report task-bind` before it is processed.

### Classroom

```bash
go run ./cmd/assistent course list
go run ./cmd/assistent course add <course-id-or-url>
go run ./cmd/assistent course tasks <course-id-or-url>
go run ./cmd/assistent course remove <course-id>
```

`course add` accepts a numeric course ID, a full Google Classroom URL, or the Base64URL course segment from a Classroom URL. Removing a course disables polling but preserves historical records. `classroom` remains an alias for compatibility.

### CLI scope

The current Cobra CLI exposes `report`, `course`, and teacher Telegram binding. Other teacher management, notification retry, dead-letter, and history functionality exists in application/storage layers but does not currently have CLI commands.

To bind a teacher's private Telegram chat, list the users of the specific Classroom course and use the local teacher ID:

```bash
go run ./cmd/assistent teacher list <course-id-or-url>
go run ./cmd/assistent teacher telegram-bind <teacher-id-or-classroom-user-id>
```

Teacher names and emails are read from the selected course through the Classroom API. The one-time link stores a Telegram channel for the selected teacher after they open it.

## First-time setup

The application does not process arbitrary courses or tasks automatically. Configure the relationship explicitly:

```bash
# Stop the poller first if its process lock is held.
go run ./cmd/assistent course add <course-id-or-url>
go run ./cmd/assistent report configure <course-id>

# Discover coursework.
just run

# Stop the poller if needed, inspect tasks, and bind the desired task.
go run ./cmd/assistent report task-list
go run ./cmd/assistent report task-bind <task-id>

# Resume polling.
just run
```

The task ID is the persisted internal ID, not a guessed Classroom title.

## Report pipeline

```text
Google Classroom
    -> submissions and material metadata
    -> retrieval and parser registry
    -> normalized parsed documents
    -> 28-appendix aggregation
    -> conflicts, missing submissions, invalid materials, evidence
    -> persisted report result
    -> Google Sheet or local XLSX
```

Missing appendices remain represented in the 28-appendix structure. Numeric values default to zero, text/list values remain empty where appropriate, and supporting evidence is kept separate.

The Google Sheets renderer creates a spreadsheet with sections for appendices, supporting evidence, invalid materials, missing submissions, and conflicts. The local renderer creates an XLSX-compatible workbook with the same sections.

## Development commands

```bash
just test
go test ./...

just coverage
go test ./... -coverpkg=./cmd/assistent,./internal/... -coverprofile=coverage.out
go tool cover -func=coverage.out

go vet ./...
go test -race ./...
git diff --check
```

CI enforces at least 90% total coverage. Tests should be based on measured uncovered code.

## Justfile commands

| Command | Purpose |
|---|---|
| `just run` | Start the background poller |
| `just test` | Run the Go test suite |
| `just coverage` | Generate coverage output and `coverage.html` |
| `just build` | Build `bin/assistent` |
| `just hermes-build` | Build the local Hermes image |
| `just hermes-up` | Build and start Hermes with Docker Compose |
| `just hermes-status` | Show Hermes Compose status |
| `just hermes-setup` | Install Graphify and test Hermes GitHub MCP access |
| `just hermes-connect-codex` | Add OpenAI Codex authentication to Hermes |
| `just hermes-tasks` | Run the Hermes delivery loop for issues 8–16 |
| `just hermes-tasks-range start end` | Run the Hermes loop for an explicit range |

Hermes commands are development automation and are not required to run the report application.

## Project structure

```text
cmd/assistent/                 executable entry point
internal/domain/               entities, rules, and ports
internal/application/          use-case orchestration and polling
internal/adapters/classroom/   Google Classroom API integration
internal/adapters/drive/       Drive retrieval and OAuth
internal/adapters/parser/      document parsers
internal/adapters/renderer/    Google Sheets and local XLSX renderers
internal/adapters/storage/     SQLite repositories
internal/platform/config/      typed environment configuration
internal/platform/database/    SQLite schema and migrations
internal/platform/locking/     single-process lock
internal/fxmodules/            Uber Fx dependency wiring
internal/presentation/cobra/   CLI commands and formatting
mocks/                         generated test mocks
tests/support/fixtures/        shared fixtures
templates/scientific-report/   28-appendix reference templates
docs/                          integration documentation
```

## Architecture

The project uses four layers:

```text
Presentation -> Application -> Domain
       |            |          ^
       +------------+-- Infrastructure adapters
```

Domain ports define external boundaries. Application services coordinate use
cases through those ports. Infrastructure implements Google, SQLite, filesystem,
parser, renderer, and notification integrations. Cobra parses commands and calls
application services. Uber Fx constructs the dependency graph.

Domain and application code do not import Google API models, SQLite rows, or
Cobra DTOs. See [ARCHITECTURE.md](ARCHITECTURE.md).

## Persistence

SQLite is the single operational state store. The default database is:

```text
data/report-system.db
```

It stores scenarios, Classroom courses, tasks, bindings, polling checkpoints,
submissions, materials, report runs, outputs, versioned results, deliveries,
reminders, teachers, channels, and processing history. Disabling a course or
scenario preserves historical records.

See [database.md](database.md) for the schema.

## Testing

The default suite does not call live Google services. External integrations use
fakes, HTTP test servers, and isolated SQLite databases. Tests cover domain
behavior, application orchestration, persistence, mappers, parsers, renderers,
and Cobra commands.

See [TESTING.md](TESTING.md) for the testing strategy.

## Documentation

- [ARCHITECTURE.md](ARCHITECTURE.md) — layers, ports, Fx, and design rules.
- [SYSTEM_DESCRIPTION.md](SYSTEM_DESCRIPTION.md) — product behavior and workflow.
- [database.md](database.md) — SQLite schema and persistence rules.
- [TESTING.md](TESTING.md) — testing and coverage expectations.
- [docs/drive.md](docs/drive.md) — Drive authorization and retrieval.
- [docs/hermes.md](docs/hermes.md) — Hermes automation.

## Security

Never commit OAuth client secrets, refresh tokens, `.env`,
`spreadsheet_secret.json`, the SQLite database, downloaded materials, or
generated report artifacts. Rotate credentials immediately if a secret is exposed.

