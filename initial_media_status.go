package main

import (
	"fmt"
	"strings"
)

// normalizeInitialMediaResultV85 prevents the cheap metadata-only comparison
// from making a semantic claim that only Smart Media Guard can prove. For
// photos/videos, name and byte size are only candidate-discovery metadata:
// renamed files, resized/re-encoded media and collision suffixes are common.
// Without a comparable remote hash, the safe initial state is POSSIBLE until
// Smart Media Guard has checked the actual content.
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
		return
	}

	// A same-size local candidate must never be presented as MISSING merely
	// because its filename is unrelated. This is the exact failure mode seen
	// with downloaded images carrying generated/collision suffixes: AllDup can
	// prove identical bytes while a filename-first pass would otherwise say
	// that the remote file is absent. Keep it in review until the content guard
	// proves duplicate/different.
	if r.Status == "MISSING" && r.Candidates > 0 {
		r.Status = "POSSIBLE"
		r.Confidence = "Provizoriu • aceeași dimensiune • necesită Smart Media Guard"
		r.Reason = fmt.Sprintf("Există %d fișier(e) media local(e) cu exact aceeași dimensiune, dar numele nu oferă o potrivire sigură. Numele diferit nu dovedește că fișierul lipsește; Smart Media Guard trebuie să verifice efectiv conținutul înainte de verdict.", r.Candidates)
	}
}
