# TEST87 local preview single-winner

- Direct image preview stays the fast path.
- If the 1.25 s watchdog expires, DDG cancels the direct `<img>` request before starting Blob fallback.
- Every async callback is generation-guarded and stale callbacks are ignored.
- A successful preview is terminal for that generation: later timeout/error callbacks cannot replace it.
- The currently displayed local image is the only element allowed to mutate the local preview UI.
- Existing MIME sniffing, Range probe, local-only fallback and 7 s fallback timeout remain.
- No HDD rescan, MEGA scan or JDownloader handoff behavior is changed.
