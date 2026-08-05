## Decision 17
Phase: 4
Subphase: P4.0 / P4.4
Question
--------
How do we fingerprint file content for metadata sync (not for chunked P2P yet)?

Options
-------
A. Size + mtime only (no content hash)
B. SHA-256 of whole file
C. BLAKE3 of whole file
D. Content-defined chunk hashes (Rabin) from day one

Decision
--------
B for Phase 4 metadata: **SHA-256** of the full file contents, hex-encoded.
Also record `size` and `mtime` as hints for skipping rehash when unchanged.

Chunking / BLAKE3 / CDC remain Phase 5 concerns (ADR 7). Phase 4 only needs
“did this path’s bytes change?” for the metadata log.

Reason
------
Size+mtime alone false-positives on copy tools and false-negatives on some
touch-ups. SHA-256 is ubiquitous, easy to explain in interviews, and already
in the Go stdlib (`crypto/sha256`) — no new dependency. BLAKE3 is faster but
adds a dep before we need streaming chunk pipelines. CDC is transfer-layer,
not metadata-identity for v1.
