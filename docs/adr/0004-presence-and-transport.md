## Decision 4
Question
--------
How do devices advertise online/offline presence to the coordinator in Phase 1,
and what realtime transport do we commit to?

Options
-------
A. Long-lived WebSocket; presence = connection liveness
B. HTTP heartbeat / polling (POST /presence/heartbeat every N seconds)
C. Both: HTTP CRUD now, WebSocket added when metadata fan-out needs it
D. Server-Sent Events only

Decision
--------
C. Phase 1 uses HTTP endpoints for register/link/subscribe plus a simple
heartbeat that marks a device online and expires to offline after a TTL.
WebSocket is the intended fan-out path for later metadata sync, but is not
wired in Phase 1.

Reason
------
Presence for peer introduction only needs "is this device reachable right
now?" — a heartbeat + TTL is enough before metadata events exist. Building
WebSocket before any event stream invites unused complexity. When Phase 4
(metadata sync loop) starts, we add WS (or upgrade heartbeat sessions) so
presence and push share one connection. Polling alone is rejected as the
*long-term* realtime path; it is acceptable only as the Phase 1 presence
mechanism.
