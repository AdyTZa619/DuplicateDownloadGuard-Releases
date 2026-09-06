package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const jdownloaderDirectBaseV8550 = "http://127.0.0.1:9666"

// handleQueueAddRoutedV8550 is a fail-safe in front of the normal DDG queue.
// If the UI selected JDownloader, the request can never fall through to the
// internal queue: it is guarded and handed to JDownloader directly instead.
func (a *App) handleQueueAddRoutedV8550(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 2<<20))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var probe struct {
		Engine string `json:"engine"`
	}
	if err := json.Unmarshal(body, &probe); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if strings.EqualFold(strings.TrimSpace(probe.Engine), "jdownloader") {
		a.handleJDownloaderDirectV8550(w, r, body)
		return
	}

	r.Body = io.NopCloser(bytes.NewReader(body))
	a.handleQueueAdd(w, r)
}

func (a *App) handleJDownloaderDirectV8550(w http.ResponseWriter, r *http.Request, body []byte) {
	var req struct {
		IDs         []int  `json:"ids"`
		Destination string `json:"destination"`
		GuardMode   string `json:"guardMode"`
		AllowReview bool   `json:"allowReview"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if len(req.IDs) == 0 {
		http.Error(w, "selecție goală", http.StatusBadRequest)
		return
	}

	a.mu.RLock()
	destination := strings.TrimSpace(req.Destination)
	if destination == "" {
		destination = strings.TrimSpace(a.cfg.DownloadDir)
	}
	rows := append([]Result(nil), a.results...)
	a.mu.RUnlock()
	if destination == "" {
		destination = portableDownloadsDir()
	}

	selected := selectedResults(rows, req.IDs)
	if len(selected) == 0 {
		http.Error(w, "rezultatele selectate nu mai există", http.StatusNotFound)
		return
	}

	report, err := a.runDownloadGuard(r.Context(), selected, destination, req.GuardMode)
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	allowed := make(map[int]bool, len(report.Decisions))
	for _, decision := range report.Decisions {
		if decision.Verdict == guardDownload || (decision.Verdict == guardReview && req.AllowReview) {
			allowed[decision.ResultID] = true
		}
	}

	links := make([]string, 0, len(selected))
	descriptions := make([]string, 0, len(selected))
	seen := make(map[string]bool, len(selected))
	for _, res := range selected {
		if !allowed[res.ID] {
			continue
		}
		link := strings.TrimSpace(jdownloaderURLForResultV8545(res))
		if link == "" || seen[link] {
			continue
		}
		seen[link] = true
		links = append(links, link)
		name := strings.TrimSpace(res.Remote.Name)
		if name == "" {
			name = strings.TrimSpace(res.Remote.Path)
		}
		if name == "" {
			name = "DDG"
		}
		descriptions = append(descriptions, name)
	}

	if len(links) == 0 {
		jsonOut(w, map[string]any{
			"ok": true, "jdownloader": true, "added": 0, "externalAdded": 0,
			"destination": destination, "guard": report, "rejected": []any{},
			"message": "Nimic de trimis în JDownloader: selecția este duplicat sau necesită verificare.",
		})
		return
	}

	if err := sendJDownloaderFlashGotV8550(r, links, descriptions, destination); err != nil {
		// Fail closed: a JDownloader failure must NEVER start the DDG downloader.
		http.Error(w, "JDownloader: "+err.Error()+". DDG nu a pornit niciun download intern.", http.StatusBadGateway)
		return
	}

	jsonOut(w, map[string]any{
		"ok": true, "jdownloader": true, "added": 0, "externalAdded": len(links),
		"destination": destination, "guard": report, "rejected": []any{},
		"message": fmt.Sprintf("%d fișier(e) trimise exclusiv în JDownloader • %s", len(links), destination),
	})
}

func sendJDownloaderFlashGotV8550(r *http.Request, links, descriptions []string, destination string) error {
	form := url.Values{}
	form.Set("urls", strings.Join(links, "\n"))
	form.Set("description", strings.Join(descriptions, "\n"))
	form.Set("package", "Duplicate Download Guard")
	form.Set("dir", strings.TrimSpace(destination))
	form.Set("autostart", "1")

	ctx := r.Context()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, jdownloaderDirectBaseV8550+"/flashgot", strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")
	req.Header.Set("User-Agent", "DuplicateDownloadGuard/8.5-JDownloader")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("nu răspunde pe 127.0.0.1:9666 (%w)", err)
	}
	defer resp.Body.Close()
	replyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	reply := strings.TrimSpace(string(replyBytes))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if reply != "" {
			return fmt.Errorf("HTTP %d: %s", resp.StatusCode, reply)
		}
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	if strings.Contains(strings.ToLower(reply), "failed") {
		return fmt.Errorf("JDownloader a refuzat cererea: %s", reply)
	}
	return nil
}
