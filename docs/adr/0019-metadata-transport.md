## Decision 19
Phase: 4
Subphase: P4.0 / P4.1 / P4.5
Question
--------
For Phase 4 metadata delivery, do we require WebSockets immediately?

Options
-------
A. HTTP push + pull only (POST events, GET since=seq); poll on an interval
B. WebSocket fan-out from day one of Phase 4
C. HTTP pull only when the user opens the app

Decision
--------
A for the Phase 4 vertical slice. Devices:
1. Push outbox events with `POST /v1/folders/{id}/events`
2. Pull with `GET /v1/folders/{id}/events?since=seq`
3. While `deviceagent run` is up, poll pull on a short interval (and after
   each successful push)

WebSocket fan-out upgrades later (still aligned with ADR 4) once the event
schema and apply path are proven. Presence heartbeat remains separate.

Reason
------
Proving oplog + cursor + LWW apply is the hard correctness work. Adding WS
before that couples transport debugging with sync bugs. Polling while the
agent runs is acceptable for a personal multi-device MVP; WS is an
optimization, not a prerequisite for metadata correctness.
