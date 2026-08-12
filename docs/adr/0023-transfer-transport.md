## Decision 23
Phase: 5
Subphase: P5.0 / P5.2
Question
--------
What transport do devices use for byte transfer in Phase 5 MVP, and what
connectivity problems do we defer?

Options
-------
A. Direct TCP listen/dial using advertised `host:port` (LAN / reachable IPs)
B. WebRTC data channels with STUN/TURN from day one
C. QUIC / HTTP/3 custom stack
D. Always tunnel file bytes through the coordination server

Decision
--------
A for MVP. Each `deviceagent` binds a TCP port (configurable), advertises
it via presence (ADR 21), and accepts authenticated pull requests
(ADR 22).

Deferred: NAT traversal, hole punching, TURN relays, WebRTC. If both
devices are not reachable (typical phone↔home-laptop over cellular),
transfer waits — consistent with product "offline waits" and no
server-side spare copy.

Reason
------
WebRTC/TURN (B) is a product unto itself and risks becoming a byte relay
(D) if we self-host TURN carelessly. QUIC (C) is fine later but adds
dependency surface before the first successful pull. A plain TCP path
proves introduction + crypto + whole-file verify on a student demo
network (two machines on the same Wi‑Fi or localhost). Documenting the
NAT limitation honestly is better than a half-broken hole-punch.
