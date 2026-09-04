package main

import "testing"

func TestQueueSnapshotIdentityDoesNotTrustReusedResultID(t *testing.T) {
	job := &DownloadJob{ResultID: 12, Remote: RemoteItem{Source: "MEGA", URL: "https://mega.nz/folder/one", Handle: "AAA", Name: "one.mp4", Size: 100}}
	same := Result{ID: 99, Remote: RemoteItem{Source: "MEGA", URL: "https://mega.nz/folder/one", Handle: "AAA", Name: "one.mp4", Size: 100}}
	if !queueJobMatchesResultV854(job, same) {
		t.Fatal("persistent snapshot should match the same remote even when display ResultID changed")
	}
	reusedID := Result{ID: 12, Remote: RemoteItem{Source: "MEGA", URL: "https://mega.nz/folder/two", Handle: "BBB", Name: "other.mp4", Size: 100}}
	if queueJobMatchesResultV854(job, reusedID) {
		t.Fatal("same ResultID must never make a different remote look identical")
	}
}

func TestLegacyQueueIdentityRequiresStoredSourceToMatch(t *testing.T) {
	job := DownloadJob{ResultID: 5, Source: "HTTP", Name: "a.jpg", URL: "https://cdn.test/a.jpg", BytesTotal: 10}
	good := Result{ID: 5, Remote: RemoteItem{Source: "HTTP", Name: "a.jpg", Size: 10, URL: "https://cdn.test/a.jpg", DirectURL: "https://cdn.test/a.jpg"}}
	if !legacyQueueJobMatchesResultV854(job, good) {
		t.Fatal("matching legacy HTTP job should be recoverable")
	}
	bad := Result{ID: 5, Remote: RemoteItem{Source: "HTTP", Name: "b.jpg", Size: 10, URL: "https://cdn.test/b.jpg", DirectURL: "https://cdn.test/b.jpg"}}
	if legacyQueueJobMatchesResultV854(job, bad) {
		t.Fatal("reused legacy ResultID must not match a different URL/name")
	}
}

func TestDecisionForQueueJobUsesRemoteIdentity(t *testing.T) {
	job := &DownloadJob{ResultID: 1, Remote: RemoteItem{Source: "HTTP", URL: "https://cdn.test/right.mp4", DirectURL: "https://cdn.test/right.mp4", Name: "right.mp4", Size: 50}}
	selected := []Result{
		{ID: 1, Remote: RemoteItem{Source: "HTTP", URL: "https://cdn.test/wrong.mp4", DirectURL: "https://cdn.test/wrong.mp4", Name: "wrong.mp4", Size: 50}},
		{ID: 2, Remote: RemoteItem{Source: "HTTP", URL: "https://cdn.test/right.mp4", DirectURL: "https://cdn.test/right.mp4", Name: "right.mp4", Size: 50}},
	}
	decisions := map[int]DownloadGuardDecision{1: {ResultID: 1, Verdict: guardDuplicate}, 2: {ResultID: 2, Verdict: guardDownload}}
	d, ok := decisionForQueueJobV854(job, selected, decisions)
	if !ok || d.ResultID != 2 || d.Verdict != guardDownload {
		t.Fatalf("decision must follow source identity, got %+v ok=%v", d, ok)
	}
}
