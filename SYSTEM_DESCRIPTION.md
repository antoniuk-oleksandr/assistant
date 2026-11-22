# Scheduled Report Aggregation System

## 1. Purpose

Scheduled Report Aggregation System is a local application for collecting, validating, aggregating, and distributing periodic departmental submissions.

The current and only supported workflow is scientific activity reporting. The
system is configured around a registered Classroom course and a report task;
there is no user-selectable scenario layer.

The application:

- schedules bound report tasks from their persisted Classroom deadlines;
- tracks report tasks explicitly bound to registered courses;
- reads submitted materials;
- parses DOCX, XLSX, PDF, Google Docs, Google Sheets, and images through interchangeable interfaces;
- aggregates numeric and textual data;
- creates a departmental report package with one aggregate Google Sheet built from submitted materials mapped to the known set of 28 appendices and their supporting files;
- reminds people who have not submitted their work;
- retries failed notifications and stores exhausted failures in a SQLite dead-letter table;
- writes daily local logs.

## 2. Core concepts

### Report workflow

The report workflow is the built-in scientific activity aggregation process. It
collects Classroom submissions, parses the known appendix structure, renders a
single departmental result, and sends configured notifications. Users do not
register, enable, disable, or select scenarios.

### Task

A task is an external assignment, currently a Google Classroom `CourseWork` item.

### Course

The application stores the selected Google Classroom course as a normalized numeric `course_id`. The CLI accepts either a full Classroom URL or the Base64URL segment used by Classroom URLs. For example:

```text
https://classroom.google.com/c/OTAwNDIzOTQ1Mjk3
                         └── Base64URL → 900423945297
```

The selected course is persisted in SQLite and is the only course polled by the
report workflow. The application must not scan every course visible to the OAuth
account unless explicitly requested.

The course commands behave as follows:

```text
course add <reference>
```

`reference` may be a full Classroom URL, a numeric course ID, or the Base64URL
segment from a Classroom URL. The command extracts the `/c/` segment when a URL
is supplied, decodes Base64URL when necessary, validates that the result is a
numeric course ID, and stores the normalized ID in SQLite. The original
reference is retained for audit purposes. Adding an already registered course
is idempotent and does not create a duplicate.

```text
course list
```

Lists registered courses with their normalized ID, original reference, status,
and registration time.

```text
course remove <course-id>
```

Disables the registered course and stops polling it. It does not erase its
tasks, submissions, report runs, or audit history. A course can be registered
again without losing historical data.

### Submission

A submission is one staff member's reporting package for the task. It may contain any subset of the 28 known appendices, plus supporting evidence such as certificates, publication pages, patents, or other files. For reminder purposes, a submission is either submitted or not submitted. File completeness and material access are validated separately.

For each discovered task, the application persists the expected Classroom
student roster. Classroom user IDs are the stable identity used to match a
submission; names and emails are copied from the profile only when available.

Each polling cycle reconciles that roster with submitted Classroom user IDs and
persists the currently missing participants, including notification state.

The first aggregate report is created when a task enters the persisted
`three_days_before` phase. A task still in discovery is collected but does not
create a preliminary report; the report-run and result version records make a
repeated poll idempotent.

When the task enters `one_day_before`, polling reprocesses the current
submissions in a new report run, preserving the prior aggregate and its source
records while incorporating late submissions.

### Appendix set

The scientific reporting scenario knows a fixed set of 28 possible appendices. The system processes every appendix that is actually submitted. Missing appendices are represented in the aggregate with scenario-defined defaults: zero for numeric counters, a dash or absence marker for scalar fields, and an empty collection for lists. The aggregate still follows the 28-appendix structure.

The reference office templates are stored in:

```text
templates/scientific-report/
├── appendix-01.docx
├── appendix-02.docx
├── ...
└── appendix-28.docx
```

The numeric filename is the stable appendix identifier. Compound identifiers
remain explicit, for example `appendix-12-1.docx` and `appendix-15-3.docx`.
Template files are reference inputs for parser mapping and output generation;
they are not assumed to be submitted by every staff member.

### Report run

A `ReportRun` is one processing lifecycle for one report-task binding.

### Result

A result is a `ReportPackage` produced through a renderer interface. Its primary human-readable output is one aggregate Google Sheet containing the departmental summary. It contains aggregated values extracted from the Google Docs and other materials actually submitted by staff, mapped to the known set of 28 appendices, plus source information, missing or invalid submission information, supporting files, and delivery metadata. The package is stored locally and may be exported to Excel when required by the official process.

## 3. Report model

The system is organized around:

```text
Registered course → Report task → Result
```

The report workflow:

```text
Scientific report workflow
├── collects staff submissions
├── processes appendices and supporting materials
├── aggregates numeric and textual indicators
├── identifies missing submissions
├── creates the departmental report
└── sends reminders and the final result
```

The report workflow is compiled into the application. It is configured through
the report commands and does not require scenario registration or a YAML/JSON
configuration file. Configure the course before binding a task:

```text
report configure <course-id>
report data-set <course-id> --field name=value [--field name=value ...]
report data-list <course-id>
report task-bind <task-id>
report task-unbind <task-id>
report task-list

course list
course add <course-url-or-id>
course tasks <course-url-or-id>
course remove <course-id>
```

An unconfigured course or unbound task is not processed. The explicit report
course and task relationships are authoritative.

Course-specific report metadata is stored after report configuration.
`report data-set` accepts repeatable `name=value` fields and updates
only the named fields; `report data-list` reads the persisted values. Common
keys include `department_name` (кафедри), `department_email`,
`lab_assistant_name`, `lab_assistant_mobile_phone`, and `department_phone`.
The command only writes SQLite; report generation is a separate operation.
Writes are rejected for an inactive or unconfigured course.

Example:

```bash
go run ./cmd/assistent report data-set <course-id> \
  --field department_name="Department name" \
  --field department_email=department@example.com \
  --field lab_assistant_name="Anna Kovalenko" \
  --field lab_assistant_mobile_phone="+380..." \
  --field department_phone="+380..."
```

## 4. Background process

The application is distributed as one executable and starts its background loop by default:

```text
report-system.exe
```

There are no separate `worker once` or `worker run` modes.

Startup sequence:

```text
load .env
initialize SQLite
acquire process lock
read last_check_timestamp
load bound report tasks and their deadlines
wait for the next deadline event
```

Only one process may run at a time. A second instance exits with `ApplicationAlreadyRunning`. The checkpoint is advanced only after a polling cycle completes successfully, so a restart can resume from the last completed check.

## 5. Google Classroom integration

The application uses Google Classroom API through OAuth 2.0, authorized by an account that can view all submissions in the target course. A teacher account for the course is the preferred authorization identity.

The integration uses:

```text
courses.courseWork.list
courses.courseWork.studentSubmissions.list
courses.courseWork.studentSubmissions.get
```

Required access normally includes:

- `classroom.student-submissions.students.readonly` for submissions in courses the user teaches or administers;
- `classroom.profile.emails` for matching a Classroom user to a notification profile;
- Drive/Docs/Sheets read access for retrieving submitted material.

Polling performs the following steps:

1. discover new or changed coursework;
2. find tasks explicitly bound to the configured report workflow;
3. retrieve `StudentSubmission` resources;
4. store the student ID, email, state, submission time, and attachment metadata;
5. download or read available Drive, Docs, or Sheets materials, including the 28 appendices and supporting files;
6. pass each material to the parser registry.

Submission access does not automatically guarantee attachment access. If the submission is visible but an attachment is not accessible, the system keeps the submitted state and marks the material as `ACCESS_DENIED`, `NOT_FOUND`, or `FAILED`.

## 6. Document ingestion and parser interfaces

Retrieving a Classroom submission and parsing its content are separate operations.

```text
ClassroomSubmissionSource
    ↓
SubmittedMaterial
    ↓
DocumentParser
    ↓
ParsedDocument
```

### Submission source

```text
SubmissionSource
├── list_tasks()
├── get_task(task_id)
├── list_submissions(task_id)
└── get_submission(submission_id)
```

The Classroom implementation retrieves the submitter, state, timestamps, and attachments. It does not interpret document contents.

### Parser interface

```text
DocumentParser
├── can_handle(material) -> Boolean
├── parse(material) -> ParsedDocument
└── capabilities() -> ParserCapabilities
```

Parser implementations may include:

```text
DocxParser
XlsxParser
PdfTextParser
PdfTableParser
GoogleDocsParser
GoogleSheetsParser
ImageOcrParser
```

The parser registry selects a parser by MIME type, extension, source type, document structure, and optionally appendix type.

### Unknown-material analysis

Staff may submit files whose names and formats do not identify their purpose.
The ingestion pipeline therefore supports an LLM-assisted fallback:

```text
Downloaded material
    ↓
Known parser, when applicable
    ↓ otherwise
LLM material analyzer
    ↓
Structured extraction proposal
    ↓
Schema validation and confidence checks
    ↓
Aggregation
```

The LLM analyzer may classify a file, identify its appendix, extract candidate
fields, and mark it as supporting evidence. Its response must use a versioned
structured schema, for example:

```json
{
  "appendix": 3,
  "document_type": "publication_evidence",
  "fields": {
    "title": "Distributed Systems Research",
    "publication_count": 1,
    "pages": 12
  },
  "is_supporting_evidence": true,
  "confidence": 0.91
}
```

The LLM never writes directly to Google Sheets and never decides final numeric
totals. Application code validates the proposal, preserves its source and
confidence, detects conflicts, and performs aggregation. Low-confidence or
ambiguous proposals become warnings or conflicts for review. Original files and
the extraction proposal are retained locally for auditability.

Every parser returns the same normalized model:

```text
ParsedDocument
├── document_id
├── source_uri
├── source_format
├── author
├── submitted_at
├── document_type
├── appendix_number
├── fields[]
├── tables[]
├── attachments[]
├── warnings[]
└── confidence
```

Every extracted field keeps its normalized name, original label, value type, value, and source location such as page, table, row, or cell.

## 6.1. Google API contract

The first implementation uses these Google Workspace REST resources. The Go
adapters must keep these provider models inside Infrastructure and map them to
Domain models.

### OAuth token exchange

```text
POST https://oauth2.googleapis.com/token
```

The OAuth client library performs the authorization-code exchange and refresh
flow. The application stores the refresh token securely and sends the access
token as:

```text
Authorization: Bearer <access-token>
```

### List visible courses

```text
GET https://classroom.googleapis.com/v1/courses
```

Query parameters:

```text
pageSize=100
pageToken=<nextPageToken>
```

Response fields used:

```json
{
  "courses": [
    {
      "id": "900423945297",
      "name": "test-1",
      "section": "...",
      "description": "...",
      "courseState": "ACTIVE"
    }
  ],
  "nextPageToken": "..."
}
```

The production application must filter this response by courses registered via
the CLI. It must not process every visible course by default.

### List coursework

```text
GET https://classroom.googleapis.com/v1/courses/{courseId}/courseWork
```

Query parameters:

```text
pageSize=100
pageToken=<nextPageToken>
```

Response fields used:

```json
{
  "courseWork": [
    {
      "courseId": "900423945297",
      "id": "123456789",
      "title": "Науковий звіт",
      "description": "...",
      "state": "PUBLISHED",
      "creationTime": "2026-11-19T10:00:00Z",
      "updateTime": "2026-11-19T10:00:00Z",
      "dueDate": { "year": 2026, "month": 11, "day": 21 },
      "dueTime": { "hours": 23, "minutes": 59 }
    }
  ],
  "nextPageToken": "..."
}
```

### List student submissions

```text
GET https://classroom.googleapis.com/v1/courses/{courseId}/courseWork/{courseWorkId}/studentSubmissions
```

Query parameters:

```text
pageSize=100
pageToken=<nextPageToken>
```

Response fields used:

```json
{
  "studentSubmissions": [
    {
      "courseId": "900423945297",
      "courseWorkId": "123456789",
      "id": "submission-1",
      "userId": "user-1",
      "state": "TURNED_IN",
      "late": false,
      "creationTime": "2026-11-19T10:30:00Z",
      "updateTime": "2026-11-19T10:35:00Z",
      "assignmentSubmission": {
        "attachments": [
          {
            "driveFile": {
              "driveFile": {
                "id": "drive-file-1",
                "title": "report.docx",
                "alternateLink": "https://drive.google.com/..."
              }
            }
          }
        ]
      }
    }
  ],
  "nextPageToken": "..."
}
```

The reminder decision uses the submission `state`. The system preserves the
submitted state even when an attachment cannot be downloaded.

### Read Drive file metadata

```text
GET https://www.googleapis.com/drive/v3/files/{fileId}
```

The adapter requests:

```text
fields=id,name,mimeType,size,webViewLink,capabilities
```

### Download Drive file content

```text
GET https://www.googleapis.com/drive/v3/files/{fileId}?alt=media
```

The response body is binary content and is saved locally before parsing. A
403/404 response becomes a material status such as `ACCESS_DENIED` or
`NOT_FOUND`; it must not change a previously observed submitted state.

### Required OAuth scopes

```text
https://www.googleapis.com/auth/classroom.courses.readonly
https://www.googleapis.com/auth/classroom.student-submissions.students.readonly
https://www.googleapis.com/auth/classroom.profile.emails
https://www.googleapis.com/auth/drive.readonly
```

## 7. Validation and aggregation

Only validated materials enter aggregation. Invalid or unreadable materials remain linked to the submission and produce a warning or error.

Numeric values:

- a dash is treated as zero where the field is numeric;
- values from valid submissions are summed or counted according to the report rules;
- every result keeps its source materials.

Text values:

- are merged without silent overwriting;
- keep their original source and author;
- may be deduplicated according to the report rules.

If a merge conflict is detected, the system:

1. stores both values and their sources;
2. creates a `DataConflict` record;
3. marks the result as requiring review;
4. notifies both parties involved;
5. does not silently choose a value.

## 8. Report output

Output is produced through an interface:

```text
ReportRenderer
├── render(report_data) -> ReportPackage
└── save(package, output_directory) -> saved package
```

The input consists of all Google Docs and other materials actually submitted by staff. The primary output is one Google Sheet with aggregate values in the corresponding cells, including defaults for missing appendices and the task appearance date and submission deadline in the header. Supporting evidence files are preserved and associated with their source submission. The local package is the source of truth for delivery, and the aggregate sheet may be exported to Excel. JPG/PDF files are evidence attachments, not the report format.

## 9. Reminder subsystem

Teachers are identified primarily by the email address of their Google account. A profile may have multiple active notification channels.

```text
teacher list <course-id-or-url>
teacher show <teacher-id>
teacher edit <teacher-id> ...
teacher enable <teacher-id>
teacher disable <teacher-id>
teacher remove <teacher-id>
teacher phone-set <teacher-id-or-classroom-user-id> --phone <phone-number>
teacher channel enable <channel-id>
teacher channel disable <channel-id>
```

Notification providers share one interface:

```text
NotificationProvider
├── send(message, recipient) -> DeliveryResult
├── validate(recipient) -> ValidationResult
├── available() -> Boolean
└── provider_name() -> String
```

Initial providers are email and Telegram. Disabled teachers and channels remain in the database and are not used for delivery.

Reminder timing:

- three days before the deadline, create a preliminary result and remind users without a submitted Classroom task;
- notify the responsible person about missing submitters;
- one day before the deadline, poll Classroom again and update the result;
- after the deadline, send the final result.

Delivery failures are stored in SQLite. Automatic retries occur after 5, 10, and 30 seconds. After three failures, the notification moves to the dead-letter table.

```text
notification retry <id>
notification dlq list
notification dlq retry-all
```

## 10. SQLite and persistence

SQLite is the single source of operational state. The detailed schema is documented in [database.md](database.md).

The database stores scenarios, tasks, scenario bindings, polling checkpoints, teachers, channels, submissions, materials, report runs, outputs, notification deliveries, dead-letter notifications, conflicts, and audit events.

## 11. Report run lifecycle

```text
ReportRun
├── id
├── scenario
├── task
├── started_at
├── finished_at
├── status
├── received_submissions
├── missing_submissions
├── result
└── error
```

Possible states are `scheduled`, `collecting`, `processing`, `waiting_for_deadline`, `completed`, and `failed`.

The run model provides recovery after restart, auditability, and idempotent processing.

## 12. Environment and deployment

Secrets and machine-specific values are loaded from environment variables, typically through a local `.env` file:

```text
GOOGLE_CREDENTIALS_PATH
GOOGLE_CLASSROOM_ID
SMTP_HOST
SMTP_PORT
SMTP_USERNAME
SMTP_PASSWORD
SMTP_FROM
REPORT_READY_EMAIL_TEMPLATE
REMINDER_TEMPLATE_PATH
TELEGRAM_BOT_TOKEN
TELEGRAM_BASE_URL
TELEGRAM_CHAT_ID
SQLITE_DATABASE_PATH
LOG_DIRECTORY
```

After a Google Sheet is generated, each active Classroom participant with an
email address receives a report-ready email through Google SMTP. Its body is
loaded from `templates/report-ready-email.txt` unless
`REPORT_READY_EMAIL_TEMPLATE` is set. The `<google_sheet_link>` tag is replaced
with the generated Sheet URL. Configure a Google app password in
`SMTP_PASSWORD`, use `smtp.gmail.com` on port `587`, and set `SMTP_FROM` to
the authenticated Gmail address (or leave it empty to use `SMTP_USERNAME`).
Report recipients come from active Classroom participants stored in SQLite;
their email addresses are populated by the Classroom API. After the report
email is sent, the same rendered message is sent to `TELEGRAM_CHAT_ID` when an
active recipient has a phone channel configured. The phone channel acts as the
admin's Telegram notification enablement; the phone number itself is not sent
to Telegram.

Telegram delivery uses `TELEGRAM_BOT_TOKEN` and defaults to
`https://api.telegram.org`; `TELEGRAM_BASE_URL` can point to a test server.
`TELEGRAM_CHAT_ID` supplies the default Telegram destination when a delivery
target does not provide one. An explicit teacher channel destination still
takes precedence. Providers are disabled by configuration in local tests and
no external calls are made unless credentials are present.

At the three-day-before-deadline phase, missing roster participants receive one
Google SMTP email reminder when their profile has an email address. The reminder
contains the task title, due date, remaining time, and Classroom task link. Its
default template is `templates/missing-submission-reminder.txt`; the path can be
overridden with `REMINDER_TEMPLATE_PATH`. Reminder records retain the rendered
subject/body and delivery status in SQLite, and the unique task/participant/
channel/kind key prevents duplicate reminders.

`GOOGLE_CLASSROOM_ID` is the normalized numeric course ID. The preferred way to register it is:

```text
course add https://classroom.google.com/c/OTAwNDIzOTQ1Mjk3
```

This stores `900423945297`, not the encoded URL segment.

Classroom task lifecycle metadata is stored separately: provider creation and
update timestamps, local discovery and last-seen timestamps, due date, due
time, and `GOOGLE_CLASSROOM_TIMEZONE` used to interpret the due fields. The
default timezone is UTC when the variable is not configured.

Each task also stores one idempotent lifecycle phase: `discovery`,
`three_days_before`, `one_day_before`, `deadline_reached`, or `completed`, plus
the timestamp at which that phase was reached. The phase names are retained for
compatibility, but their windows are configurable through
`REPORT_PRELIMINARY_BEFORE` and `REPORT_REFRESH_BEFORE`. Values accept `d`,
`h`, `m`, and `s` suffixes, so `5m` and `2m` can be used for local testing.
The scheduler does not poll on a fixed interval. It waits until
`due_at - REPORT_PRELIMINARY_BEFORE` to build the first report, then
`due_at - REPORT_REFRESH_BEFORE` to fetch and rebuild only when submissions
changed, and finally `due_at` to disable the task binding. It does not build a
new file at the deadline.
Re-running the poller while a task remains in the same phase does not change
the phase timestamp.

The `.env` file is never written to reports or logs and is not committed to version control.

Recommended layout:

```text
report-system/
├── report-system.exe
├── .env
├── data/
│   └── report-system.db
└── logs/
    ├── 2026-11-19.log
    └── 2026-11-20.log
```

Logs are local, daily files. They contain startup, polling checkpoints, discovered tasks and submissions, state transitions, parser results, notifications, retries, dead-letter entries, conflicts, and API/provider/database errors.

Once per month, the application removes old operational data according to retention rules. Open conflicts, unresolved dead-letter entries, and active-period results are not removed automatically.

## 13. First version scope

The first version must demonstrate one complete path:

1. discover a Classroom task;
2. bind it to `ScienceReportScenario`;
3. retrieve submissions;
4. parse at least one supported appendix format;
5. aggregate data;
6. create the 28-appendix departmental report package and save its supporting files locally;
7. detect missing submissions;
8. send a reminder and a final notification;
9. deliver the package and its attachments through the configured email/messenger channels;
10. recover correctly after restarting the application.

The design must keep all external integrations behind interfaces so new scenarios, parsers, renderers, storage providers, and notification providers can be added without rewriting the application core.
