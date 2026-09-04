package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const contentGraphSchemaV860 = 1

type ContentGraphNodeV860 struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"` // remote|local
	Identity  string `json:"identity"`
	Source    string `json:"source,omitempty"`
	Path      string `json:"path,omitempty"`
	Name      string `json:"name,omitempty"`
	Size      int64  `json:"size,omitempty"`
	MTime     int64  `json:"mtime,omitempty"`
	HashType  string `json:"hashType,omitempty"`
	Hash      string `json:"hash,omitempty"`
	UpdatedAt int64  `json:"updatedAt"`
}

type ContentGraphEdgeV860 struct {
	From      string `json:"from"`
	To        string `json:"to"`
	Relation  string `json:"relation"` // downloaded-from|exact-content|same-source|version-of
	Evidence  string `json:"evidence,omitempty"`
	Score     int    `json:"score,omitempty"`
	UpdatedAt int64  `json:"updatedAt"`
}

type ContentGraphV860 struct {
	Schema    int                             `json:"schema"`
	Nodes     map[string]ContentGraphNodeV860 `json:"nodes"`
	Edges     map[string]ContentGraphEdgeV860 `json:"edges"`
	UpdatedAt int64                           `json:"updatedAt"`
}

var contentGraphMuV860 sync.Mutex

func contentGraphPathV860(a *App) string {
	if a == nil || strings.TrimSpace(a.appDir) == "" {
		return ""
	}
	return filepath.Join(a.appDir, "content_graph.json")
}

func contentGraphIDV860(kind, identity string) string {
	h := sha256.Sum256([]byte(strings.TrimSpace(kind) + "\x00" + strings.TrimSpace(identity)))
	return strings.TrimSpace(kind) + ":" + hex.EncodeToString(h[:12])
}

func contentGraphEdgeKeyV860(from, to, relation string) string {
	return from + "|" + to + "|" + relation
}

func newContentGraphV860() ContentGraphV860 {
	return ContentGraphV860{Schema: contentGraphSchemaV860, Nodes: map[string]ContentGraphNodeV860{}, Edges: map[string]ContentGraphEdgeV860{}}
}

func loadContentGraphV860(a *App) ContentGraphV860 {
	g := newContentGraphV860()
	p := contentGraphPathV860(a)
	if p == "" {
		return g
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return g
	}
	var disk ContentGraphV860
	if json.Unmarshal(b, &disk) != nil || disk.Schema != contentGraphSchemaV860 {
		return g
	}
	if disk.Nodes == nil {
		disk.Nodes = map[string]ContentGraphNodeV860{}
	}
	if disk.Edges == nil {
		disk.Edges = map[string]ContentGraphEdgeV860{}
	}
	return disk
}

func saveContentGraphV860(a *App, g ContentGraphV860) error {
	p := contentGraphPathV860(a)
	if p == "" {
		return errors.New("cale content graph indisponibilă")
	}
	g.Schema = contentGraphSchemaV860
	g.UpdatedAt = time.Now().Unix()
	b, err := json.MarshalIndent(g, "", "  ")
	if err != nil {
		return err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, b, 0644); err != nil {
		return err
	}
	return replaceCacheFileV85(tmp, p)
}

func normalizedRemoteHashV860(r RemoteItem) (string, string) {
	t := strings.ToLower(strings.TrimSpace(r.HashType))
	h := strings.ToLower(strings.TrimSpace(r.Hash))
	if h == "" || (t != "sha256" && t != "md5") {
		return "", ""
	}
	return t, h
}

func localGraphNodeV860(path string) (ContentGraphNodeV860, error) {
	st, err := os.Stat(path)
	if err != nil || st.IsDir() {
		if err == nil {
			err = errors.New("cale locală este folder")
		}
		return ContentGraphNodeV860{}, err
	}
	clean, _ := filepath.Abs(path)
	identity := strings.ToLower(filepath.Clean(clean))
	return ContentGraphNodeV860{
		ID: contentGraphIDV860("local", identity), Kind: "local", Identity: identity,
		Path: clean, Name: filepath.Base(clean), Size: st.Size(), MTime: st.ModTime().UnixNano(), UpdatedAt: time.Now().Unix(),
	}, nil
}

func remoteGraphNodeV860(r RemoteItem) (ContentGraphNodeV860, error) {
	identity := stableRemoteKeyV855(r)
	if identity == "" {
		return ContentGraphNodeV860{}, errors.New("identitate remote stabilă indisponibilă")
	}
	ht, hv := normalizedRemoteHashV860(r)
	return ContentGraphNodeV860{
		ID: contentGraphIDV860("remote", identity), Kind: "remote", Identity: identity,
		Source: r.Source, Path: r.Path, Name: r.Name, Size: r.Size, HashType: ht, Hash: hv, UpdatedAt: time.Now().Unix(),
	}, nil
}

func recordDownloadedContentGraphV860(a *App, remote RemoteItem, localPath string) error {
	remoteNode, err := remoteGraphNodeV860(remote)
	if err != nil {
		return err
	}
	localNode, err := localGraphNodeV860(localPath)
	if err != nil {
		return err
	}
	contentGraphMuV860.Lock()
	defer contentGraphMuV860.Unlock()
	g := loadContentGraphV860(a)
	g.Nodes[remoteNode.ID] = remoteNode
	g.Nodes[localNode.ID] = localNode
	now := time.Now().Unix()
	edge := ContentGraphEdgeV860{From: remoteNode.ID, To: localNode.ID, Relation: "downloaded-from", Evidence: "download verified by DDG", Score: 100, UpdatedAt: now}
	g.Edges[contentGraphEdgeKeyV860(edge.From, edge.To, edge.Relation)] = edge
	if remoteNode.Hash != "" {
		exact := ContentGraphEdgeV860{From: remoteNode.ID, To: localNode.ID, Relation: "exact-content", Evidence: remoteNode.HashType + ":" + remoteNode.Hash, Score: 100, UpdatedAt: now}
		g.Edges[contentGraphEdgeKeyV860(exact.From, exact.To, exact.Relation)] = exact
	}
	return saveContentGraphV860(a, g)
}

func contentGraphStatsV860(a *App) map[string]any {
	contentGraphMuV860.Lock()
	defer contentGraphMuV860.Unlock()
	g := loadContentGraphV860(a)
	relations := map[string]int{}
	for _, e := range g.Edges {
		relations[e.Relation]++
	}
	keys := make([]string, 0, len(relations))
	for k := range relations {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	ordered := map[string]int{}
	for _, k := range keys {
		ordered[k] = relations[k]
	}
	return map[string]any{"schema": g.Schema, "nodes": len(g.Nodes), "edges": len(g.Edges), "relations": ordered, "updatedAt": g.UpdatedAt}
}
