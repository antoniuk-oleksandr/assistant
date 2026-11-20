---
name: hermes-workflow
description: Orchestrate the project Developer, Tester, Reviewer workflow.
---

# Workflow

The parent agent orchestrates every implementation task and delegates sequentially:

1. `hermes-dev` implements production code.
2. `hermes-tester` adds or corrects tests without changing production behavior.
3. `hermes-reviewer` reviews the complete diff and is read-only.

Reviewer findings are classified as `production_fix`, `test_fix`, `both`, or `human_blocker`. Fixes return to the responsible role, followed by another review. Use at most three review-fix cycles. After approval, the parent orchestrator owns the delivery lifecycle: commit the complete change, push the branch, open and merge the linked pull request, verify it is closed, update the GitHub Project item to Done, then return the local checkout to `main` and pull before starting the next item.
