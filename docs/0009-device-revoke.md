## Decision 9
Question
--------
What does "revoke a device" mean on the coordinator, and how does it interact
with tokens and presence?

Options
-------
A. Delete the device row (CASCADE tokens, subscriptions, presence)
B. Soft-revoke: set `revoked_at`; reject auth; keep row for audit / history
C. Delete tokens only; leave the device row active (can re-auth somehow)
D. Account-wide logout (invalidate every device)

Decision
--------
B. Soft-revoke.

- `devices.revoked_at` set (non-null = revoked)
- All `auth_tokens` for that device deleted immediately
- Auth middleware rejects revoked devices even if a stale token somehow remained
- Presence forced to `offline` and heartbeats from that device rejected
- Subscriptions retained (metadata intent) but inactive until a future
  re-link policy; Phase 2 does not allow un-revoke — re-link = new device
  registration / pairing flow producing a new DeviceID
- A device may revoke *another* device on the same account; self-revoke
  (logout) is allowed and clears the local keystore on the client

Reason
------
Hard-delete loses useful history (name, last seen, public key) that helps the
user recognize "which phone did I just kick?". Token-only revoke without a
device flag is ambiguous if a new token is later issued to the same DeviceID.
Account-wide logout is a separate action we do not need yet. Soft-revoke +
token wipe is the smallest correct model for "lost phone" and "this laptop
logout".
