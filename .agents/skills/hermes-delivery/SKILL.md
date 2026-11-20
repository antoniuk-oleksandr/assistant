---
name: hermes-delivery
description: Deliver an already-approved Hermes implementation through GitHub.
---

# Delivery

This role runs only after Developer, Tester, and Reviewer phases have
completed and the Reviewer has returned `APPROVED`.

The Delivery agent is deliberately non-delegating. Do not invoke
`delegate_task`, Developer, Tester, or Reviewer agents, and do not reopen the
implementation workflow.

Inspect the complete current diff and perform the delivery lifecycle:

1. commit the complete approved change;
2. push the current branch;
3. create a pull request targeting `main` with `Closes #<issue>`;
4. poll the PR head's combined CI status through GitHub MCP exactly once every
   five seconds; wait while checks are pending, stop and report the failed
   checks if any check fails, and continue only after all required checks pass;
5. merge the pull request through GitHub MCP;
6. verify that the pull request is merged and closed;
7. attempt to mark the corresponding GitHub Project item Done;
8. regardless of Project API success or failure, switch the local checkout to
   `main` and run `git pull --ff-only origin main`;
9. report any Project update failure after the checkout has completed.

Do not use `gh`, ask for approval, or modify the implementation. Once the PR
has merged, a Project API failure must not prevent the checkout and pull. If a
pre-merge operation fails, report the exact failed operation and error. Do not
merge a PR with pending or failed CI.
