# Triage labels

Use these labels consistently across GitHub issues in `1123786563/myqypt`.

| Label | Meaning | Apply when | Remove when |
| --- | --- | --- | --- |
| `needs-triage` | The request has not been classified or routed. | A new issue needs initial review. | Scope, ownership, readiness, and next action are clear. |
| `needs-info` | Work cannot proceed without missing information. | A concrete question, reproduction, decision, credential, or dependency is required from a human. | The missing information has been supplied and incorporated. |
| `ready-for-agent` | An agent can execute the issue autonomously. | Scope and acceptance criteria are clear, dependencies are available, and no human-only action blocks progress. | Work is claimed, becomes blocked, needs human action, or is completed. |
| `ready-for-human` | The next action must be performed or decided by a human. | Approval, product judgment, credentials, external coordination, or another human-only step is required. | The human action is complete and the issue can move to another state. |
| `wontfix` | The request will not be implemented. | The decision and rationale are recorded and the issue should be closed without implementation. | The decision is explicitly reversed. |

## Rules

- Every newly discovered request starts with `needs-triage` unless it is classified immediately.
- Do not apply `ready-for-agent` and `needs-info` at the same time.
- Do not apply `ready-for-agent` and `ready-for-human` at the same time.
- Use `ready-for-human` for a known human next action; use `needs-info` when the required input is missing or unclear.
- When applying `wontfix`, record the reason before closing the issue.
- Labels describe the current routing state, not historical states. Remove stale labels when the state changes.
