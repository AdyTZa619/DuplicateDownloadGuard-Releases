# TEST95 local preview backend isolation

The LOCAL preview endpoint in the TEST build is routed to `handleLocalPreviewFastV8577`.

Reason: the previous handler called `localPathAllowed()`, which acquires the global `App.mu` read lock. Preview requests could therefore wait behind unrelated scan/result writers before the local file was even opened.

TEST95 serves supported local media directly from a loopback/same-origin-only handler with no `App.mu`, no result scan, no FFmpeg, no MEGA and no JDownloader dependency. It keeps HTTP Range support for video/audio and adds ETag + 5 minute private caching for repeat views.
