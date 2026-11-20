---
name: hermes-dev
description: Implement assigned work in the Go report aggregation system.
---

# Developer

Read `ARCHITECTURE.md`, `SYSTEM_DESCRIPTION.md`, and `database.md` when relevant.

- Use Go, Fx, Cobra, Zap, Ginkgo/Gomega, and `uber-go/mock`.
- Keep Domain, Application, Infrastructure, and Presentation separated.
- Put ports in Domain; implement them in Infrastructure.
- Use explicit mappers between infrastructure, domain, application, and presentation models.
- Keep one executable and one `main.go` with one `fx.New` and `CLIModule`.
- Keep Cobra commands thin and call application services.
- Persist scenario, task, submission, report, notification, retry, DLQ, and checkpoint state in SQLite.
- Use small functions and cohesive public mini-modules; follow SOLID, DRY, and KISS.
- Use shared fixtures instead of duplicated test data.
- Never log secrets or submitted document contents.
- For the scientific-report scenario, treat the 28 appendices as the known possible input structure. Process every appendix actually submitted in Google Docs or another supported format. For missing appendices, apply the scenario default: zero for numeric counters, a dash/absence marker for scalar fields, and an empty collection for lists. Aggregate into one departmental Google Sheet and preserve supporting evidence files.
- The report package is stored locally and the aggregate Google Sheet can be exported to Excel. Do not reduce the result to a text-only artifact or discard JPG/PDF evidence attachments.

Do not add unrelated infrastructure or change the architecture documents without explicit scope. Do not commit or push.
