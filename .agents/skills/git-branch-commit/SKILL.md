---
name: git-branch-commit
description: Create consistent Git branches and commit messages for project work. Use when implementation work needs branch naming, commits, or PR preparation.
---

# Git branches and commits

Use this skill for branch and commit naming. It does not authorize pushing, creating pull requests, merging, or changing GitHub. Those actions require explicit authorization in the current task.

## Branch names

Create branches from the latest `main` branch. Use lowercase kebab-case and include the issue number when one exists:

```text
<type>/issue-<number>-<short-description>
```

Preferred branch types:

- `feat/` for a user-facing feature;
- `fix/` for a bug fix;
- `chore/` for maintenance or repository work;
- `ci/` for GitHub Actions or other CI changes.

Examples:

```text
feat/issue-16-scientific-submissions
fix/issue-23-missing-submission-state
chore/issue-8-update-fixtures
ci/issue-17-test-coverage
```

Without an issue number, use `<type>/<short-description>`.

## Commit messages

Use one of these prefixes followed by simple, specific text:

```text
feat: add scenario registration
fix: preserve submitted state when a file is inaccessible
chore: update shared test fixtures
ci: require 90 percent test coverage
```

Use these prefixes:

- `feat:` for a new user-facing capability;
- `fix:` for correcting incorrect behavior;
- `chore:` for maintenance that does not change product behavior;
- `ci:` for continuous-integration workflow changes.

Prefer an imperative verb such as `add`, `support`, `preserve`, `prevent`, `update`, or `require`. Keep the subject concise, do not add a period, and avoid implementation details unless they clarify the change.

If a change contains multiple unrelated concerns, split it into separate commits. Do not make empty commits, include secrets, or commit generated artifacts unless the project explicitly tracks them.

## Issue and PR linking

When the work belongs to a GitHub issue, include its number in the commit or PR context when useful. A PR body should close the issue explicitly:

```text
Closes #16
```

Use the same concise feature description in the PR title as in the main commit, for example:

```text
feat: add scientific submission processing
```

Before committing, inspect the diff and run the checks required by the repository instructions. Commit only the intended feature changes.
