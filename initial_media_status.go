package main

import "strings"

// normalizeInitialMediaResultV85 prevents the cheap metadata-only comparison
// from making a semantic claim that only Smart Media Guard can prove. For
// photos/videos, same display name with a different byte size can be a resize,
// re-encode, container change or genuinely different file. Without a remote
// hash, the safe initial state is POSSIBLE, not DIFFERENT.
func normalizeInitialMediaResultV85(r *Result) {
	if r == nil {
		return
	}
	kind := remoteMediaKind(r.Remote.Name)
	if kind != "image" && kind != "video" {
		return
	}
	if strings.TrimSpace(r.Remote.Hash) != "" {
		return
	}
	if r.Status == "DIFFERENT" && r.LocalPath != "" && r.NameScore == 100 {
		r.Status = "POSSIBLE"
		r.Confidence = "Provizoriu • necesită Smart Media Guard"
		r.Reason = "Există același nume local, dar mărimea diferă. Pentru media aceasta poate însemna re-encode, redimensionare, container sau altă versiune; verdictul final necesită verificarea conținutului."
	}
}
