---
name: go-architecture
description: Apply the project's Go, Fx, Cobra, layers, ports, mappers, and interfaces.
---

# Go Architecture

Use the architecture defined in `ARCHITECTURE.md`.

```text
Domain → Application → Infrastructure
                         ↑
                    Presentation
```

Ports live in Domain. Infrastructure implements them. Presentation contains Cobra, DTOs, input parsing, output formatting, and exit codes. Models never cross boundaries without explicit mappers.

Use Fx for all construction, one `main.go`, one `fx.New`, and one `CLIModule`. Use interfaces at integration boundaries and public mini-modules for reusable behavior.
