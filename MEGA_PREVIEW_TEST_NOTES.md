# MEGA Preview 8.5.11-test.1

This is a Windows validation build, not a stable release. It does not update
`VERSION`, `update.json`, `SHA256SUMS.txt`, or the stable executable.

## Root cause found in 8.5.3–8.5.10

- MEGAcmd script commands are clients of one background `MEGAcmdServer.exe`.
  Cold startup and session resume happen inside that server; adding more client
  commands cannot make it a persistent DDG service.
- A new scan stopped preview and performed `session -> logout -> login -> find ->
  webdav /`. MEGAcmd documents that logout removes WebDAV locations and its local
  session cache, so this invalidated the root that 8.5.10 tried to preserve.
- 8.5.10 accepted a remembered root when its TCP port answered. This did not
  prove that the same MEGAcmd session and served root were still behind the port.
- Browser `onerror` represented both transport failures and unsupported codecs.
  Every error could add a per-file WebDAV location even when the root delivered
  bytes correctly.
- Temporary per-file fallbacks were not tracked while the canonical state stayed
  on `/`. They therefore accumulated in MEGAcmd's persisted
  `webdav_served_locations`, matching the degradation after repeated previews.
- Two JavaScript wrappers owned fallback behavior, making a second fallback
  possible and obscuring the route that actually failed.

## Test architecture

- One controller owns one public-folder session and one whole-folder WebDAV root.
- Same-source rescans reuse that session/root and do not logout/login.
- A remembered root is accepted only after both a MEGAcmd `webdav` listing and an
  HTTP response verify it; a TCP listener alone is rejected.
- Browser errors are diagnosed with HEAD and a one-byte Range GET. If bytes are
  available, DDG reports a codec/container limitation and does not add a node.
- Compatibility fallback is limited to one tracked per-file node. The previous
  fallback is removed before a different one is added.
- Every click gets an `MP-xxxxxx` trace. The journal records route, sanitized
  commands, command duration, HTTP status, root/fallback state, Windows server
  process and listener details, and the fallback reason.

## Windows validation checklist

1. Start the test EXE with MEGAcmd fully stopped and preview 10 mixed files.
2. Record the first click and clicks 2–10, including their `MP-xxxxxx` IDs.
3. Change preview at least 50 times. Confirm later clicks do not grow slower.
4. Scan the same MEGA link again. Confirm the journal says `SCAN REUSE` and no
   logout/login is executed.
5. Preview an MKV/AVI unsupported by Edge. It must offer VLC without creating a
   per-file fallback when the Range probe succeeds.
6. Test after DDG restart, after stopping `MEGAcmdServer.exe`, and after changing
   the MEGAcmd session externally.
7. If transfer quota is exhausted, confirm the UI says `Cotă MEGA depășită` and
   retain the trace ID from the journal.

The result is not considered fixed until this checklist passes on real Windows
with the user's MEGAcmd installation.
