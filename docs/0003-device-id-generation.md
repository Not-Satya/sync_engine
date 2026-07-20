## Decision 3
Question
--------
How is DeviceID generated and how stable is it?

Options
-------
A. Random UUIDv4 assigned by the server at registration
B. Client-generated UUIDv4, server stores as primary key
C. Derived from a device keypair (hash/fingerprint of public key), Syncthing-style
D. Hardware identifiers (MAC, Android ID, etc.)

Decision
--------
C for the identifier shape; Phase 1 issues a server-side Ed25519 keypair at
device registration and sets DeviceID = first 32 chars of base32(SHA-256(pubkey)),
with the public key persisted. Private key material is returned once to the
client (in-memory handoff for now); durable client keystore arrives in a later
phase. Until then the device also authenticates with its opaque bearer token.

Reason
------
Hardware IDs are unstable and privacy-hostile across reinstalls. Pure random
UUIDs work but give no path to peer authentication during P2P transfer.
Key-derived IDs keep DeviceID stable across reinstalls *when the client keeps
the key*, and they unlock mutual auth for peer transfer without redesign.
Phase 1 wires the ID format and schema; full P2P handshake and client-side
key persistence are deferred.
