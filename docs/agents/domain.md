# Domain documentation

This repository uses a single-context domain documentation structure.

## Before making domain changes

1. Read the root `CONTEXT.md` or `CONTEXT-MAP.md` if present.
2. Read the ADRs under `docs/adr/` that are relevant to the requested change.
3. Use the documented glossary and domain vocabulary in code, APIs, events, schemas, and user-facing explanations.
4. Check whether the proposed change conflicts with an accepted ADR.

If a context document or relevant ADR does not exist, proceed silently using the best available repository evidence. Do not block routine work merely because documentation is absent.

## File structure

- `CONTEXT.md`: the canonical bounded-context description, glossary, invariants, and ownership boundaries.
- `CONTEXT-MAP.md`: optional context relationships if the repository later grows beyond one context.
- `docs/adr/NNNN-short-title.md`: architecture decision records.

## Change discipline

- Keep domain objects owned by their documented product or platform boundary.
- Update `CONTEXT.md` when established terminology, invariants, ownership, or context boundaries change.
- Add a new ADR for a durable architectural decision; do not rewrite the outcome of an accepted historical ADR.
- Flag conflicts with accepted ADRs before implementation and make the required decision explicit.
- Keep implementation naming aligned with the glossary. If an external system uses different terms, isolate the translation at the integration boundary.
