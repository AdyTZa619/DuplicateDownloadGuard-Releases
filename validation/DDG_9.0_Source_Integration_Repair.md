# DDG 9.0 source detector integration repair

The previous TEST exposed the rich detector in download preflight but did not run it after the primary source scan. This change connects `compareRemote` to a cancellable sequential background worker and publishes each decision into the existing results API and evidence view.

Name and size alone now retain a review candidate. Cryptographic matches immediately expose EXACT evidence. Measured scores replace legacy name/size scores when detector evidence exists. Similarities from 60 to 84 remain SIMILAR; incomplete candidate searches retain their measured score and pending count with an insufficient-data classification. Download history alone no longer fabricates 100% content confidence.

The worker snapshots the local index once, shares the bounded detector, waits for the source operation to release MEGA, and uses a separate generation fence so cancellation, clearing, or replacement cannot apply an old result to a new source. It releases its MEGA queue lease without stopping the existing preview. The results panel exposes progress, cancellation, and retry. The lifecycle integration only cancels this worker during shutdown.

## Verification

- Regression reproduced before repair: primary comparison returned VERIFIED with a nil detector evidence object.
- Actual `/api/url/scan` through `/api/results`: automatically verifies renamed identical bytes without calling preflight.
- Blocked HTTP content: status remains responsive; replacement cancels the old request and preserves the replacement source.
- 19 real encoded FFmpeg/image corpus cases through the source HTTP API: 19 passed, about 54.5 seconds on this Linux workspace. Includes 3 exact, 13 transformed, and 3 negative cases. The existing corpus uses synthetic source imagery; this is not a native-camera or user-library benchmark.
- Go suite passed, targeted source tests passed under the race detector, Windows-targeted vet passed, and all 8 JavaScript behavior tests passed locally.
- Windows CI and publication are separate required gates after this commit.

## Limits

The corpus uses one local candidate per case and does not establish exhaustive recall or throughput on a large cold library. Existing bounded candidate budgets remain; unfinished searches are explicitly reported and can be retried. No authenticated MEGA account, real Windows desktop interaction, or user's HDD collection is available in this workspace. This repair does not claim those acceptance checks were performed.

Scope: duplicate engine and minimal source/results/shutdown integration; no Media Picker change or general redesign. Target is testing only.
