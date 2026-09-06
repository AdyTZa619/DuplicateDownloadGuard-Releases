package main

import (
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"strings"
)

type jdCrawlJobV8545 struct {
	Text           string `json:"text"`
	Enabled        string `json:"enabled"`
	AutoStart      string `json:"autoStart"`
	AutoConfirm    string `json:"autoConfirm"`
	PackageName    string `json:"packageName"`
	DownloadFolder string `json:"downloadFolder,omitempty"`
}

func jdownloaderURLForResultV8545(x Result) string {
	r := x.Remote
	if strings.EqualFold(strings.TrimSpace(r.Source), "GOFILE") && strings.TrimSpace(r.ProviderID) != "" {
		if u, err := url.Parse(strings.TrimSpace(r.URL)); err == nil {
			parts := strings.Split(strings.Trim(u.Path, "/"), "/")
			if len(parts) >= 2 && strings.EqualFold(parts[0], "d") && strings.TrimSpace(parts[1]) != "" {
				q := url.Values{}
				q.Set("c", strings.TrimSpace(parts[1]))
				return "https://gofile.io/?" + q.Encode() + "#file=" + url.QueryEscape(strings.TrimSpace(r.ProviderID))
			}
		}
	}
	return resultDownloadURL(x)
}

func writeJDownloaderCrawlJobV8545(path string, urls []string, downloadFolder string) error {
	jobs := make([]jdCrawlJobV8545, 0, len(urls))
	seen := make(map[string]bool, len(urls))
	for _, raw := range urls {
		u := strings.TrimSpace(raw)
		if u == "" || seen[u] {
			continue
		}
		seen[u] = true
		jobs = append(jobs, jdCrawlJobV8545{
			Text:           u,
			Enabled:        "TRUE",
			AutoStart:      "FALSE",
			AutoConfirm:    "FALSE",
			PackageName:    "Duplicate Download Guard",
			DownloadFolder: strings.TrimSpace(downloadFolder),
		})
	}
	if len(jobs) == 0 {
		return errors.New("nu există linkuri JDownloader de scris")
	}
	b, err := json.MarshalIndent(jobs, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	return os.WriteFile(path, b, 0644)
}
