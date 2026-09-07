# TEST92 local preview

- Restores the pre-TEST85 fast LOCAL preview path for normal files: one stable `/api/local-preview` request.
- Images use the direct browser/WebView decode first; there is no cache-busting, Blob copy, full-file fetch, or thumbnail generation on the fast path.
- Video/audio remain direct and use `preload="metadata"`, preserving Range-capable `http.ServeFile` behavior.
- Only an image that errors or remains unresolved for 4 seconds switches once to `/api/local-thumb`.
- The direct image request is stopped before the fallback starts, preventing two HDD reads from racing.
- LOCAL preview code contains no MEGA scan/preview calls, no index start, and no JDownloader preflight.
- Focused Windows validation passed on the final direct-path implementation: JS syntax, Go tests, vet, and Windows x64 build.
