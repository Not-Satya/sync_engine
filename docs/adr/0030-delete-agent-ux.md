## Decision 30
Phase: 6
Subphase: P6.0 / P6.4
Question
--------
How does the deviceagent expose sync-delete vs remove-local-copy (and undo)?

Options
-------
A. Only filesystem gestures (delete file vs … nothing for omit)
B. CLI under `deviceagent folders …` for path ops
C. CLI under `deviceagent files …` with explicit verbs; FS delete remains
   sync-delete via the running agent
D. Desktop UI only

Decision
--------
**C** for Phase 6.

New command group (paths relative to a bound folder):

```
deviceagent files remove-local -folder-id ID -path REL
deviceagent files materialize  -folder-id ID -path REL
```

- `remove-local`: ADR 29 omit + unlink; requires keystore/bindings/index.
- `materialize`: clear omit + pull bytes (needs online peer if absent).

Sync delete stays: user deletes the file in Explorer/Finder/`rm` while
`deviceagent run` is watching — scanner emits `MetaOpDelete` (ADR 27).
No separate `files delete` in v1 unless we need a headless path later.

`folders status` gains `omitted=N` (count of live `omit_local=1` rows)
alongside existing `missing=N` (want blob, don’t have it, not omitted).

Reason
------
Omit must be explicit so it cannot be confused with sync delete. Putting
path ops under `files` keeps `folders *` about FolderID ↔ path ↔ subscribe
(ADR 14). Desktop UI (D) is post-MVP packaging; the CLI is the contract the
agent already uses for everything else.
