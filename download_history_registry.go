package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

type downloadHistoryEntryV85 struct {
	Key        string `json:"key"`
	Source     string `json:"source"`
	Name       string `json:"name"`
	Bytes      int64  `json:"bytes"`
	OutputPath string `json:"outputPath"`
	FinishedAt int64  `json:"finishedAt"`
	FileSize   int64  `json:"fileSize"`
	FileMTime  int64  `json:"fileMtime"`
}

type queueHistoryJobV85 struct {
	Name       string `json:"name"`
	Source     string `json:"source"`
	URL        string `json:"url"`
	Status     string `json:"status"`
	BytesDone  int64  `json:"bytesDone"`
	BytesTotal int64  `json:"bytesTotal"`
	OutputPath string `json:"outputPath"`
	FinishedAt int64  `json:"finishedAt"`
}

var downloadHistoryStateV85 = struct {
	sync.RWMutex
	Loaded     bool
	Entries    map[string]downloadHistoryEntryV85
	QueueMTime int64
}{}

var downloadHistoryWatcherOnceV85 sync.Once

func downloadHistoryFileV85() string {
	return filepath.Join(executableDir(), "data", "download_history.json")
}

func downloadQueueFileV85() string {
	return filepath.Join(executableDir(), "data", "download_queue.json")
}

func downloadHistoryKeyV85(source, rawURL, name string, size int64) string {
	source = strings.ToUpper(strings.TrimSpace(source))
	rawURL = strings.TrimSpace(rawURL)
	name = strings.ToLower(strings.TrimSpace(name))
	identity := rawURL
	if identity == "" {
		identity = "name:" + name
	}
	s := source + "\x1f" + identity + "\x1f" + strconv.FormatInt(size, 10)
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func loadDownloadHistoryV85() {
	downloadHistoryStateV85.Lock()
	defer downloadHistoryStateV85.Unlock()
	if downloadHistoryStateV85.Loaded {
		return
	}
	downloadHistoryStateV85.Loaded = true
	downloadHistoryStateV85.Entries = map[string]downloadHistoryEntryV85{}
	b, err := os.ReadFile(downloadHistoryFileV85())
	if err != nil {
		return
	}
	var rows []downloadHistoryEntryV85
	if json.Unmarshal(b, &rows) != nil {
		return
	}
	for _, row := range rows {
		if row.Key != "" {
			downloadHistoryStateV85.Entries[row.Key] = row
		}
	}
}

func saveDownloadHistoryV85() error {
	loadDownloadHistoryV85()
	downloadHistoryStateV85.RLock()
	rows := make([]downloadHistoryEntryV85, 0, len(downloadHistoryStateV85.Entries))
	for _, row := range downloadHistoryStateV85.Entries {
		rows = append(rows, row)
	}
	downloadHistoryStateV85.RUnlock()
	b, err := json.MarshalIndent(rows, "", "  ")
	if err != nil {
		return err
	}
	path := downloadHistoryFileV85()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0644); err != nil {
		return err
	}
	return replaceCacheFileV85(tmp, path)
}

func syncCompletedQueueToHistoryV85() {
	loadDownloadHistoryV85()
	queuePath := downloadQueueFileV85()
	st, err := os.Stat(queuePath)
	if err != nil || st.IsDir() {
		return
	}
	mtime := st.ModTime().UnixNano()
	downloadHistoryStateV85.RLock()
	seenMTime := downloadHistoryStateV85.QueueMTime
	downloadHistoryStateV85.RUnlock()
	if mtime == seenMTime {
		return
	}
	b, err := os.ReadFile(queuePath)
	if err != nil {
		return
	}
	var jobs []queueHistoryJobV85
	if json.Unmarshal(b, &jobs) != nil {
		return
	}
	changed := false
	downloadHistoryStateV85.Lock()
	for _, job := range jobs {
		if job.Status != "completed" || strings.TrimSpace(job.OutputPath) == "" {
			continue
		}
		fileInfo, statErr := os.Stat(job.OutputPath)
		if statErr != nil || fileInfo.IsDir() {
			continue
		}
		size := job.BytesTotal
		if size <= 0 {
			size = job.BytesDone
		}
		if size <= 0 {
			size = fileInfo.Size()
		}
		key := downloadHistoryKeyV85(job.Source, job.URL, job.Name, size)
		finished := job.FinishedAt
		if finished <= 0 {
			finished = fileInfo.ModTime().Unix()
		}
		row := downloadHistoryEntryV85{
			Key:        key,
			Source:     job.Source,
			Name:       job.Name,
			Bytes:      size,
			OutputPath: job.OutputPath,
			FinishedAt: finished,
			FileSize:   fileInfo.Size(),
			FileMTime:  fileInfo.ModTime().UnixNano(),
		}
		if old, ok := downloadHistoryStateV85.Entries[key]; !ok || old.OutputPath != row.OutputPath || old.FileMTime != row.FileMTime || old.FileSize != row.FileSize || old.FinishedAt != row.FinishedAt {
			downloadHistoryStateV85.Entries[key] = row
			changed = true
		}
	}
	downloadHistoryStateV85.QueueMTime = mtime
	downloadHistoryStateV85.Unlock()
	if changed {
		_ = saveDownloadHistoryV85()
	}
}

func ensureDownloadHistoryWatcherV85() {
	downloadHistoryWatcherOnceV85.Do(func() {
		syncCompletedQueueToHistoryV85()
		go func() {
			ticker := time.NewTicker(500 * time.Millisecond)
			defer ticker.Stop()
			for range ticker.C {
				syncCompletedQueueToHistoryV85()
			}
		}()
	})
}

func historyRemoteURLV85(res Result) string {
	if u := strings.TrimSpace(resultDownloadURL(res)); u != "" {
		return u
	}
	if u := strings.TrimSpace(res.Remote.DirectURL); u != "" {
		return u
	}
	return strings.TrimSpace(res.Remote.URL)
}

func persistentDownloadHistoryDecisionV85(res Result) (DownloadGuardDecision, bool) {
	ensureDownloadHistoryWatcherV85()
	loadDownloadHistoryV85()
	size := res.Remote.Size
	key := downloadHistoryKeyV85(res.Remote.Source, historyRemoteURLV85(res), res.Remote.Name, size)
	downloadHistoryStateV85.RLock()
	row, ok := downloadHistoryStateV85.Entries[key]
	downloadHistoryStateV85.RUnlock()
	if !ok {
		return DownloadGuardDecision{}, false
	}
	st, err := os.Stat(row.OutputPath)
	if err != nil || st.IsDir() {
		return DownloadGuardDecision{}, false
	}
	if row.FileSize > 0 && st.Size() != row.FileSize {
		return DownloadGuardDecision{}, false
	}
	if size > 0 && !res.Remote.ApproxSize && st.Size() != size {
		return DownloadGuardDecision{}, false
	}
	// mtime is a cheap tamper/replacement guard. A changed file falls through to
	// the normal live hash/media checks rather than being trusted from history.
	if row.FileMTime != 0 && st.ModTime().UnixNano() != row.FileMTime {
		return DownloadGuardDecision{}, false
	}
	d := DownloadGuardDecision{
		ResultID:   res.ID,
		Name:       res.Remote.Name,
		Verdict:    guardDuplicate,
		Reason:     "Acest fișier apare în istoricul persistent al descărcărilor finalizate, iar fișierul rezultat există încă neschimbat.",
		LocalPath:  row.OutputPath,
		Method:     "download-history",
		Candidates: 1,
	}
	return decorateGuardDecision(d), true
}
