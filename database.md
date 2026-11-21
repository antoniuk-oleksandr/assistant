# Database schema

SQLite is the single source of state for the application. It stores scenario registration, task bindings, polling checkpoints, submissions, parsed materials, report runs, notifications, retries, conflicts and operational errors.

The database is local to the machine running `report-system.exe`.

## Design rules

- All timestamps are stored in UTC as ISO-8601 values.
- External identifiers are stored together with the external system name.
- External submissions and materials are idempotent by their provider identifiers.
- Disabling an entity changes its status; it does not delete the record.
- Hard deletion is allowed only for explicit administrative commands.
- Every retryable operation has an attempt count and next-attempt timestamp.

Course-specific scenario metadata is stored in `scenario_course_data`, keyed by
scenario, course, and field name. The CLI command
`scenario data-set <scenario-name> <course-id> --field name=value` upserts these
values; it does not generate report files. The polling/report workflow reads
the values when creating the next report.

## Tables

### `classroom_courses`

Google Classroom courses explicitly registered for polling. The incoming URL or
Base64URL segment is normalized before storage; for example,
`OTAwNDIzOTQ1Mjk3` becomes `900423945297`.

| Column | Type | Constraints | Description |
|---|---|---|---|
| `id` | INTEGER | PK | Internal identifier |
| `provider` | TEXT | NOT NULL | `google-classroom` |
| `external_course_id` | TEXT | UNIQUE, NOT NULL | Normalized Classroom course ID |
| `source_reference` | TEXT | NOT NULL | Original URL or encoded segment |
| `status` | TEXT | NOT NULL | `active` or `disabled` |
| `created_at` | TEXT | NOT NULL | Registration time |
| `updated_at` | TEXT | NOT NULL | Last change time |

Only active registered courses are polled. The application does not process
every course visible to the OAuth account by default.

### `scenarios`

Registered scenario implementations.

| Column | Type | Constraints | Description |
|---|---|---|---|
| `id` | INTEGER | PK | Internal identifier |
| `name` | TEXT | UNIQUE, NOT NULL | CLI name, e.g. `science-report` |
| `implementation_type` | TEXT | NOT NULL | Registered implementation name |
| `status` | TEXT | NOT NULL | `active` or `disabled` |
| `created_at` | TEXT | NOT NULL | Creation time |
| `updated_at` | TEXT | NOT NULL | Last change time |
| `last_run_at` | TEXT | NULL | Last started run |

If `implementation_type` is not registered in the application, `scenario add` fails with `ScenarioNotFound`.

### `tasks`

Google Classroom coursework items discovered by the application.

| Column | Type | Constraints | Description |
|---|---|---|---|
| `id` | INTEGER | PK | Internal identifier |
| `provider` | TEXT | NOT NULL | `google-classroom` |
| `external_task_id` | TEXT | NOT NULL | Classroom `courseWorkId` |
| `course_id` | TEXT | NOT NULL | Classroom course identifier |
| `title` | TEXT | NOT NULL | Task title |
| `description` | TEXT | NULL | Task description |
| `created_at_external` | TEXT | NULL | Time reported by Classroom |
| `due_at` | TEXT | NULL | Deadline |
| `discovered_at` | TEXT | NOT NULL | First local discovery time |
| `last_seen_at` | TEXT | NOT NULL | Last polling time |
| `status` | TEXT | NOT NULL | `discovered`, `bound`, `closed`, `ignored` |

Unique constraint: `(provider, course_id, external_task_id)`.

### `scenario_tasks`

Explicit binding between a task and a scenario.

| Column | Type | Constraints | Description |
|---|---|---|---|
| `scenario_id` | INTEGER | FK, NOT NULL | Scenario |
| `task_id` | INTEGER | FK, NOT NULL | Task |
| `bound_at` | TEXT | NOT NULL | Binding time |
| `unbound_at` | TEXT | NULL | Unbinding time |
| `status` | TEXT | NOT NULL | `active` or `disabled` |

Primary key: `(scenario_id, task_id)`.

### `scenario_courses`

Explicit binding between a registered course and a scenario. A task can be
bound only when its course is actively bound to the same scenario.

| Column | Type | Constraints | Description |
|---|---|---|---|
| `scenario_id` | INTEGER | FK, NOT NULL | Scenario |
| `course_id` | INTEGER | FK, NOT NULL | Registered Classroom course |
| `bound_at` | TEXT | NOT NULL | Binding time |
| `unbound_at` | TEXT | NULL | Unbinding time |
| `status` | TEXT | NOT NULL | `active` or `disabled` |

Primary key: `(scenario_id, course_id)`.

### `scenario_course_data`

Course-specific fields used to complete a scenario report template. Values are
normalized into rows so one command can set multiple fields and later commands
can update individual fields without replacing the complete course record.

| Column | Type | Constraints | Description |
|---|---|---|---|
| `scenario_id` | INTEGER | FK, NOT NULL | Scenario |
| `course_id` | INTEGER | FK, NOT NULL | Registered Classroom course |
| `field_name` | TEXT | NOT NULL | Stable field key |
| `field_value` | TEXT | NOT NULL | Stored value |
| `updated_at` | TEXT | NOT NULL | Last update time |

Primary key: `(scenario_id, course_id, field_name)`. Writes are accepted only
for an active scenario actively bound to an active course.

### `poll_state`

Persistent checkpoint for recovery after application restart.

| Column | Type | Constraints | Description |
|---|---|---|---|
| `id` | INTEGER | PK | One row for the application |
| `last_check_timestamp` | TEXT | NOT NULL | Last completed polling checkpoint |
| `updated_at` | TEXT | NOT NULL | Checkpoint update time |

The checkpoint is advanced only after a polling cycle completes successfully.

### `teachers`

Recipients identified by Google account email.

| Column | Type | Constraints | Description |
|---|---|---|---|
| `id` | INTEGER | PK | Internal identifier |
| `email` | TEXT | UNIQUE, NOT NULL | Google account email |
| `display_name` | TEXT | NULL | Name from Classroom |
| `classroom_course_id` | TEXT | NULL | Related course |
| `status` | TEXT | NOT NULL | `active` or `disabled` |
| `created_at` | TEXT | NOT NULL | Creation time |
| `updated_at` | TEXT | NOT NULL | Last change time |

### `notification_channels`

Channels configured for a teacher.

| Column | Type | Constraints | Description |
|---|---|---|---|
| `id` | INTEGER | PK | Internal identifier |
| `teacher_id` | INTEGER | FK, NOT NULL | Owner |
| `type` | TEXT | NOT NULL | `email`, `telegram`, or future provider |
| `address` | TEXT | NOT NULL | Email address, Telegram chat ID, etc. |
| `status` | TEXT | NOT NULL | `active` or `disabled` |
| `created_at` | TEXT | NOT NULL | Creation time |
| `updated_at` | TEXT | NOT NULL | Last change time |

### `submissions`

Google Classroom submission state. The state is binary for reminder purposes: submitted or not submitted.

| Column | Type | Constraints | Description |
|---|---|---|---|
| `id` | INTEGER | PK | Internal identifier |
| `task_id` | INTEGER | FK, NOT NULL | Task |
| `teacher_id` | INTEGER | FK, NULL | Matched by Google email |
| `external_submission_id` | TEXT | NOT NULL | Classroom submission identifier |
| `state` | TEXT | NOT NULL | `new`, `created`, `submitted`, `returned`, `unknown` |
| `submitted_at` | TEXT | NULL | Submission time |
| `late` | INTEGER | NOT NULL | Boolean |
| `first_seen_at` | TEXT | NOT NULL | First local discovery |
| `last_seen_at` | TEXT | NOT NULL | Last refresh |
| `raw_payload_path` | TEXT | NULL | Optional local raw response |

Unique constraint: `(task_id, external_submission_id)`.

### `materials`

Files or Google documents attached to a submission.

| Column | Type | Constraints | Description |
|---|---|---|---|
| `id` | INTEGER | PK | Internal identifier |
| `submission_id` | INTEGER | FK, NOT NULL | Submission |
| `external_material_id` | TEXT | NULL | Provider material identifier |
| `name` | TEXT | NOT NULL | Original name |
| `mime_type` | TEXT | NULL | MIME type |
| `source_uri` | TEXT | NULL | External URL |
| `local_path` | TEXT | NULL | Downloaded local path |
| `appendix_number` | INTEGER | NULL | Scientific report appendix number, 1–28 |
| `role` | TEXT | NOT NULL | `appendix` or `supporting_evidence` |
| `parser_type` | TEXT | NULL | Selected parser |
| `status` | TEXT | NOT NULL | `pending`, `parsed`, `invalid`, `access_denied`, `not_found`, or `failed` |
| `created_at` | TEXT | NOT NULL | Discovery time |

### `report_runs`

One processing attempt for one scenario-task binding.

| Column | Type | Constraints | Description |
|---|---|---|---|
| `id` | INTEGER | PK | Internal identifier |
| `scenario_id` | INTEGER | FK, NOT NULL | Scenario |
| `task_id` | INTEGER | FK, NOT NULL | Task |
| `status` | TEXT | NOT NULL | `scheduled`, `collecting`, `processing`, `waiting_for_deadline`, `completed`, `failed` |
| `started_at` | TEXT | NULL | Start time |
| `finished_at` | TEXT | NULL | Finish time |
| `is_partial` | INTEGER | NOT NULL | Boolean |
| `result_path` | TEXT | NULL | Local generated result |
| `error_message` | TEXT | NULL | Terminal error |
| `created_at` | TEXT | NOT NULL | Creation time |

Only one active run may exist for a given `(scenario_id, task_id)`.

### `report_outputs`

Generated result artifacts.

| Column | Type | Constraints | Description |
|---|---|---|---|
| `id` | INTEGER | PK | Internal identifier |
| `report_run_id` | INTEGER | FK, NOT NULL | Run |
| `format` | TEXT | NOT NULL | `google-docs`, `google-sheets`, `docx`, `xlsx`, or local package format |
| `path` | TEXT | NOT NULL | Local output path |
| `version` | INTEGER | NOT NULL | Version number |
| `created_at` | TEXT | NOT NULL | Creation time |

The report result is a locally persisted package containing the aggregate Google Sheet data, source references to submitted materials mapped to the 28 possible appendices, submission completeness information, and supporting-file metadata. The database stores local artifacts and delivery metadata. The aggregate sheet is the primary human-readable output and may be exported to Excel.

### `notification_deliveries`

Delivery state for every reminder or result notification.

| Column | Type | Constraints | Description |
|---|---|---|---|
| `id` | INTEGER | PK | Internal identifier |
| `report_run_id` | INTEGER | FK, NULL | Related run |
| `teacher_id` | INTEGER | FK, NULL | Recipient |
| `channel_id` | INTEGER | FK, NOT NULL | Channel |
| `notification_type` | TEXT | NOT NULL | `reminder`, `missing_report`, `report_ready`, `error`, `conflict` |
| `deduplication_key` | TEXT | UNIQUE, NOT NULL | Prevents duplicate delivery |
| `status` | TEXT | NOT NULL | `pending`, `sending`, `sent`, `retry_scheduled`, `dead_letter` |
| `attempts` | INTEGER | NOT NULL | Number of attempts |
| `next_attempt_at` | TEXT | NULL | Retry time |
| `last_error` | TEXT | NULL | Last provider error |
| `sent_at` | TEXT | NULL | Successful delivery time |

### `dead_letter_notifications`

Notifications that failed after three retries.

| Column | Type | Constraints | Description |
|---|---|---|---|
| `id` | INTEGER | PK | Internal identifier |
| `notification_id` | INTEGER | FK, UNIQUE, NOT NULL | Failed delivery |
| `provider` | TEXT | NOT NULL | Provider name |
| `recipient` | TEXT | NOT NULL | Target |
| `payload` | TEXT | NOT NULL | Serialized message |
| `attempts` | INTEGER | NOT NULL | Should be 3 after automatic retries |
| `last_error` | TEXT | NOT NULL | Failure reason |
| `created_at` | TEXT | NOT NULL | DLQ time |
| `resolved_at` | TEXT | NULL | Manual resolution time |

### `data_conflicts`

Conflicting values found during aggregation.

| Column | Type | Constraints | Description |
|---|---|---|---|
| `id` | INTEGER | PK | Internal identifier |
| `report_run_id` | INTEGER | FK, NOT NULL | Run |
| `field_name` | TEXT | NOT NULL | Conflicting field |
| `value_a` | TEXT | NOT NULL | First value |
| `source_a` | TEXT | NOT NULL | First source |
| `value_b` | TEXT | NOT NULL | Second value |
| `source_b` | TEXT | NOT NULL | Second source |
| `status` | TEXT | NOT NULL | `open` or `resolved` |
| `resolution` | TEXT | NULL | Resolution note |
| `created_at` | TEXT | NOT NULL | Detection time |

### `audit_events`

Operational audit events and errors.

| Column | Type | Constraints | Description |
|---|---|---|---|
| `id` | INTEGER | PK | Internal identifier |
| `event_type` | TEXT | NOT NULL | Event category |
| `entity_type` | TEXT | NULL | Related entity |
| `entity_id` | INTEGER | NULL | Related identifier |
| `severity` | TEXT | NOT NULL | `info`, `warning`, `error` |
| `message` | TEXT | NOT NULL | Human-readable message |
| `details` | TEXT | NULL | Structured details |
| `created_at` | TEXT | NOT NULL | Event time |

## Indexes

Recommended indexes:

```sql
CREATE UNIQUE INDEX ux_tasks_external
  ON tasks(provider, course_id, external_task_id);

CREATE UNIQUE INDEX ux_submissions_external
  ON submissions(task_id, external_submission_id);

CREATE INDEX ix_tasks_due_at ON tasks(due_at);
CREATE INDEX ix_tasks_last_seen_at ON tasks(last_seen_at);
CREATE INDEX ix_submissions_task_state ON submissions(task_id, state);
CREATE INDEX ix_report_runs_status ON report_runs(status);
CREATE INDEX ix_notifications_retry ON notification_deliveries(status, next_attempt_at);
CREATE INDEX ix_audit_events_created_at ON audit_events(created_at);
```

## Retention

Once per month the application removes old operational data according to the retention policy:

- old log files and `audit_events` are removed after the configured retention period;
- resolved notifications and old retry records are removed after the configured retention period;
- completed report runs and generated reports are retained longer than operational logs;
- open conflicts and unresolved DLQ records are never removed automatically.

Cleanup runs only when no active processing run is in progress.
