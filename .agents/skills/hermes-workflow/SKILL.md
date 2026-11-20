---
name: hermes-workflow
description: Orchestrate the project Developer, Tester, Reviewer workflow.
---

# Workflow

The parent agent orchestrates every implementation task and delegates sequentially:

1. `hermes-dev` implements production code.
2. `hermes-tester` adds or corrects tests without changing production behavior.
3. `hermes-reviewer` reviews the complete diff and is read-only.

Reviewer findings are classified as `production_fix`, `test_fix`, `both`, or `human_blocker`. Fixes return to the responsible role, followed by another review. Use at most three review-fix cycles. Do not commit, push, or open pull requests.
