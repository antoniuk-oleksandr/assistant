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

1. run the required final checks;
2. commit the complete approved change;
3. push the current branch;
4. create a pull request targeting `main` with `Closes #<issue>`;
5. merge the pull request through GitHub MCP;
6. verify that the pull request is merged and closed;
7. mark the corresponding GitHub Project item Done;
8. switch the local checkout to `main` and run `git pull --ff-only origin main`.

Do not use `gh`, ask for approval, or modify the implementation. If any
delivery operation fails, stop and report the exact failed operation and its
error.
