## Decision 1
Question
--------
What database should the coordination server use for accounts, devices, auth,
folders, subscriptions, and presence?

Options
-------
A. PostgreSQL — multi-writer, mature ops story, matches the reference architecture doc
B. modernc.org/sqlite (embedded file) — pure Go, zero external deps, same driver as device clients
C. Separate engines (Postgres server / SQLite clients) from day one

Decision
--------
B for v1. Single-node SQLite via modernc.org/sqlite as the coordination store.

Reason
------
Phase 1 is a coordination skeleton for a personal multi-device system, not a
multi-tenant SaaS. SQLite keeps the server runnable with one binary and no
Postgres dependency. The schema stays portable SQL so we can migrate to
Postgres later if write concurrency or multi-instance hosting demands it.
File bytes never touch this store — only coordination rows — so SQLite size
and write patterns stay small. Rejecting permanent blob tables here reinforces
that the server is not storage.
