package main

import "strings"

func hasRemoteSnapshotV854(job DownloadJob) bool {
	return strings.TrimSpace(job.Remote.Source) != "" || strings.TrimSpace(job.Remote.Name) != "" || strings.TrimSpace(job.Remote.URL) != "" || strings.TrimSpace(job.Remote.Handle) != ""
}

func legacyQueueJobMatchesResultV854(job DownloadJob, res Result) bool {
	if job.ResultID != res.ID {
		return false
	}
	if s := strings.TrimSpace(job.Source); s != "" && !strings.EqualFold(s, strings.TrimSpace(res.Remote.Source)) {
		return false
	}
	if n := strings.TrimSpace(job.Name); n != "" && !strings.EqualFold(n, strings.TrimSpace(res.Remote.Name)) {
		return false
	}
	if job.BytesTotal > 0 && res.Remote.Size > 0 && job.BytesTotal != res.Remote.Size {
		return false
	}
	// Old queue entries stored resultDownloadURL(). Requiring it to still match
	// is deliberately conservative: if a CDN URL changed, re-add the result
	// instead of attaching a legacy job to a different file that reused the ID.
	if u := strings.TrimSpace(job.URL); u != "" && u != strings.TrimSpace(resultDownloadURL(res)) {
		return false
	}
	return true
}

func queueJobMatchesResultV854(job *DownloadJob, res Result) bool {
	if job == nil {
		return false
	}
	if hasRemoteSnapshotV854(*job) {
		return decisionKey(job.Remote) == decisionKey(res.Remote)
	}
	return legacyQueueJobMatchesResultV854(*job, res)
}

func decisionForQueueJobV854(job *DownloadJob, selected []Result, decisions map[int]DownloadGuardDecision) (DownloadGuardDecision, bool) {
	for _, res := range selected {
		if !queueJobMatchesResultV854(job, res) {
			continue
		}
		decision, ok := decisions[res.ID]
		return decision, ok
	}
	return DownloadGuardDecision{}, false
}
