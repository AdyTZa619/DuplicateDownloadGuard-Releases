package main

import (
	"os"
	"strings"
	"time"
)

func sameExactRemoteHashV860(a, b RemoteItem) bool {
	at, ah := normalizedRemoteHashV860(a)
	bt, bh := normalizedRemoteHashV860(b)
	if at == "" || bt == "" || at != bt || ah != bh {
		return false
	}
	if a.Size > 0 && b.Size > 0 && a.Size != b.Size {
		return false
	}
	return true
}

func shouldReconcileDownloadedRemoteV860(downloaded, candidate RemoteItem) (bool, string) {
	a := stableRemoteKeyV855(downloaded)
	b := stableRemoteKeyV855(candidate)
	if a != "" && b != "" && a == b {
		return true, "same-source"
	}
	if sameExactRemoteHashV860(downloaded, candidate) {
		return true, "exact-hash"
	}
	return false, ""
}

func applyDownloadedResultFieldsV860(x *Result, path, evidence string, at int64) {
	if x == nil {
		return
	}
	x.LocalPath = path
	x.DownloadPath = path
	x.DownloadedAt = at
	x.Status = "HAVE"
	x.AutoStatus = "HAVE"
	x.AutoConfidence = "Descărcat și reconciliat automat"
	x.AutoReason = "Fișierul local rezultat din download corespunde prin " + evidence + "."
	x.Confidence = "Descărcat de program"
	x.Reason = "Conținut disponibil local după download; reconciliere " + evidence + "."
	x.GuardVerdict = guardDuplicate
	x.GuardMethod = "post-download-reconcile"
	x.GuardReason = "Fișier local verificat după download; " + evidence
	x.GuardAt = at
	// A reconciliation is automated evidence, not a user-authored manual mark.
	// Preserve an existing manual decision, but do not manufacture one.
	if !x.Manual {
		x.ManualStatus = ""
		x.ManualAt = 0
	}
}

func (a *App) postDownloadReconcileV860(downloaded Result, localPath string) int {
	if a == nil || strings.TrimSpace(localPath) == "" {
		return 0
	}
	if st, err := os.Stat(localPath); err != nil || st.IsDir() {
		return 0
	}
	// Ensure the new local artifact is immediately visible to future guards.
	a.addDownloadedToIndex(localPath)

	at := time.Now().Unix()
	changed := 0
	a.mu.Lock()
	for i := range a.results {
		x := &a.results[i]
		ok, evidence := shouldReconcileDownloadedRemoteV860(downloaded.Remote, x.Remote)
		if !ok {
			continue
		}
		applyDownloadedResultFieldsV860(x, localPath, evidence, at)
		changed++
	}
	a.revision.Add(1)
	a.mu.Unlock()
	if changed > 0 {
		_ = a.saveResults()
	}
	if err := recordDownloadedContentGraphV860(a, downloaded.Remote, localPath); err != nil {
		a.logf("Content Graph: nu am putut înregistra downloadul %s: %v", downloaded.Remote.Name, err)
	}
	return changed
}
