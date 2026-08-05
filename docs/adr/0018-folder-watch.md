## Decision 18
Phase: 4
Subphase: P4.0 / P4.3
Question
--------
How does the device learn about Create / Modify / Delete / Rename inside a
bound sync folder?

Options
-------
A. Periodic full scan only
B. fsnotify (or OS equivalent) with debounce + periodic reconcile scan
C. OS-specific richer APIs only (USN journal, FSEvents) from day one
D. User must click “Sync now”

Decision
--------
B. **fsnotify** on each bound folder root (recursive where the library /
platform allows), debounce bursts (e.g. 300–500ms), enqueue paths for hash.
Plus a **periodic full reconcile** (e.g. on start and every N minutes) to
catch missed events.

Renames: treat as delete+create at metadata level if the watcher does not
deliver a reliable rename pair; refine later if we get stable rename events.

Reason
------
Scan-only wastes CPU and delays sync. Pure fsnotify misses events under
load and on some network mounts — reconcile is the safety net (Syncthing
pattern). Platform-specific journals are better long-term but explode
complexity before MVP. Manual sync-only breaks the offline-first “just works”
story.
