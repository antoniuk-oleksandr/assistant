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
4. merge the pull request through GitHub MCP;
5. verify that the pull request is merged and closed;
6. attempt to mark the corresponding GitHub Project item Done;
7. regardless of Project API success or failure, switch the local checkout to
   `main` and run `git pull --ff-only origin main`;
8. report any Project update failure after the checkout has completed.

Do not use `gh`, ask for approval, or modify the implementation. Once the PR
has merged, a Project API failure must not prevent the checkout and pull. If a
pre-merge operation fails, report the exact failed operation and error.
