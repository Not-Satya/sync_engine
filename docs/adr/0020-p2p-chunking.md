## Decision 20
Phase: 5
Subphase: P5.0 / P5.3
Question
--------
How do we split file content for peer-to-peer byte transfer in Phase 5 MVP?

Options
-------
A. Fixed-size chunks (e.g. 1 MiB) with per-chunk hashes
B. Content-defined chunking (Rabin / FastCDC) from day one
C. Whole-file transfer keyed by SHA-256 (metadata hash from ADR 17);
   defer CDC and go-bsdiff
D. go-bsdiff patches only (no full-file path)

Decision
--------
C for Phase 5 MVP. Transfer unit = entire file identified by the same
`content_hash` already in the metadata oplog. Receiver requests
`GET hash=<sha256>`; sender streams bytes; receiver verifies SHA-256 and
size before renaming into the bound folder.

CDC (option B) and bsdiff (option D) remain post-MVP optimizations once
whole-file P2P works on LAN. Fixed-size chunks (A) are a reasonable middle
step later if resume becomes painful — not required to prove the
architecture.

Reason
------
Phase 4 already stamps whole-file SHA-256. Aligning transfer identity with
that hash avoids a second content-addressing scheme before the first byte
moves. ADR 5 warned not to bet the design on bsdiff alone; CDC is the
right long-term WAN story but explodes scope (chunk index, sparse files,
partial apply) before we have a working "Phone, get movie from Laptop"
demo. Whole-file + verify is the thinnest correct P2P path that still
rejects server blob storage.
