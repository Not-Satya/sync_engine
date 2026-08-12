## Decision 24
Phase: 5
Subphase: P5.0 / P5.3
Question
--------
If a whole-file transfer fails mid-stream, how do we resume?

Options
-------
A. No resume — delete partial temp file and retry the whole file
B. Byte-range resume (HTTP-like Range) with size+hash check at end
C. Chunk-level resume (requires chunking from ADR 20 option A/B)

Decision
--------
A for Phase 5 MVP. Write to a temp path under the bound folder (or app
cache), fsync, verify SHA-256, then atomic rename into place. On any
failure or hash mismatch, delete the temp file and retry later.

Byte-range or chunk resume (B/C) can follow once whole-file P2P is stable
and large-file demos demand it.

Reason
------
Resume couples to chunking. ADR 20 chose whole-file identity; inventing
ranges before the happy path works mixes two failure modes. Temp +
verify + rename already prevents half-written visible files. Retry on
the next fetch-planner tick is enough for MVP.
