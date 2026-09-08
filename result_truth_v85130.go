package main

import (
	"path/filepath"
	"sort"
	"strings"
	"unicode"
)

// strongFilenameIdentityTokensV85130 extracts provider/content identifiers that
// survive common download renames. Short words and date-like 8 digit values are
// deliberately excluded so the token is useful as evidence, never as proof.
func strongFilenameIdentityTokensV85130(name string) []string {
	base := strings.ToLower(filepath.Base(name))
	base = strings.TrimSuffix(base, filepath.Ext(base))
	parts := strings.Fields(nameSepRE.ReplaceAllString(base, " "))
	seen := map[string]bool{}
	out := make([]string, 0, 2)
	for _, token := range parts {
		if seen[token] {
			continue
		}
		digits := 0
		letters := 0
		for _, r := range token {
			switch {
			case unicode.IsDigit(r):
				digits++
			case unicode.IsLetter(r):
				letters++
			}
		}
		strongNumeric := letters == 0 && digits >= 9
		strongMixed := len(token) >= 12 && digits > 0 && letters > 0
		if !strongNumeric && !strongMixed {
			continue
		}
		seen[token] = true
		out = append(out, token)
	}
	sort.Strings(out)
	return out
}

func identityCandidatePathsV85130(name string, byIdentity map[string][]string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, 4)
	for _, token := range strongFilenameIdentityTokensV85130(name) {
		for _, path := range byIdentity[token] {
			key := pathKey(path)
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, path)
		}
	}
	return out
}

func bestResultCandidatePathV85130(remote RemoteItem, paths []string, index map[string]FileEntry) string {
	best := ""
	bestRank := -1
	for _, path := range paths {
		entry, ok := index[path]
		if !ok {
			continue
		}
		candidate := rankCandidate(remote, entry)
		if candidate.Rank > bestRank || (candidate.Rank == bestRank && strings.ToLower(path) < strings.ToLower(best)) {
			best = path
			bestRank = candidate.Rank
		}
	}
	return best
}

func uniqueCandidateCountV85130(groups ...[]string) int {
	seen := map[string]bool{}
	for _, group := range groups {
		for _, path := range group {
			seen[pathKey(path)] = true
		}
	}
	return len(seen)
}

func sameCandidatePathV85130(a, b string) bool {
	if strings.TrimSpace(a) == "" || strings.TrimSpace(b) == "" {
		return false
	}
	return pathKey(filepath.Clean(a)) == pathKey(filepath.Clean(b))
}

func indexedCandidateExistsV85130(path string, index map[string]FileEntry) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	if _, ok := index[path]; ok {
		return true
	}
	want := pathKey(filepath.Clean(path))
	for candidate := range index {
		if pathKey(filepath.Clean(candidate)) == want {
			return true
		}
	}
	return false
}

// persistedDecisionAppliesV85130 keeps genuine user decisions, but refuses to
// let stale history overrule stronger evidence from the current filesystem.
func persistedDecisionAppliesV85130(decision Decision, current Result, index map[string]FileEntry) bool {
	status := strings.ToUpper(strings.TrimSpace(decision.Status))
	auto := strings.ToUpper(strings.TrimSpace(current.Status))
	switch status {
	case "MISSING":
		if auto == "VERIFIED" || auto == "HAVE" || auto == "SAMPLED" {
			return false
		}
		if current.LocalPath != "" && (decision.LocalPath == "" || !sameCandidatePathV85130(decision.LocalPath, current.LocalPath)) {
			return false
		}
	case "DIFFERENT":
		if auto == "VERIFIED" || auto == "HAVE" || auto == "SAMPLED" {
			return false
		}
		if current.LocalPath != "" && decision.LocalPath != "" && !sameCandidatePathV85130(decision.LocalPath, current.LocalPath) {
			return false
		}
	case "HAVE":
		if decision.LocalPath != "" && !indexedCandidateExistsV85130(decision.LocalPath, index) {
			return false
		}
	}
	return true
}
