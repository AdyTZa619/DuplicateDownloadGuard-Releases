package main

import (
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
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
	QuickHash  string `json:"quickHash,omitempty"`
}

type queueHistoryJobV85 struct {
	ResultID   int    `json:"resultId"`
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
	SaveMu     sync.Mutex
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

func savedResultsFileV85() string {
	return filepath.Join(executableDir(), "data", "last_results.json.gz")
}

func downloadHistoryKeyV85(source, rawURL, name string, size int64) string {
	source = strings.ToUpper(strings.TrimSpace(source))
	rawURL = strings.TrimSpace(rawURL)
	name = strings.ToLower(strings.TrimSpace(name))
	identity := "name:" + name
	if rawURL != "" {
		switch {
		case source == "MEGA" && strings.Contains(strings.ToLower(rawURL), "/file/"):
			// A MEGA file handle identifies the node even if the display name later changes.
			identity = rawURL
		case source == "YT-DLP":
			// historyRemoteURLV85 builds this from the stable source page + provider ID.
			// Do not include the display filename: titles can be edited later.
			identity = rawURL
		default:
			// Generic extractors can expose several same-sized files from one page.
			// Keep the display name in the identity to avoid shared-page collisions.
			identity = rawURL + "\x1e" + name
		}
	}
	s := source + "\x1f" + identity + "\x1f" + strconv.FormatInt(size, 10)
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// quickFileFingerprintV85 hashes the whole file when it is small, otherwise
// five deterministic 64 KiB samples spread across the file. It is deliberately
// independent of filename and timestamps, so the history registry can detect a
// same-size replacement even when metadata was preserved.
func quickFileFingerprintV85(path string, size int64) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if size < 0 {
		if st, statErr := f.Stat(); statErr == nil {
			size = st.Size()
		}
	}
	h := sha256.New()
	_, _ = io.WriteString(h, strconv.FormatInt(size, 10))
	_, _ = io.WriteString(h, "\x00")
	const fullLimit = int64(12 << 20)
	if size <= fullLimit {
		if _, err := io.Copy(h, f); err != nil {
			return "", err
		}
		return hex.EncodeToString(h.Sum(nil)), nil
	}
	const block = int64(64 << 10)
	maxStart := size - block
	if maxStart < 0 {
		maxStart = 0
	}
	offsets := []int64{0, maxStart / 4, maxStart / 2, maxStart * 3 / 4, maxStart}
	seen := map[int64]bool{}
	buf := make([]byte, block)
	for _, off := range offsets {
		if seen[off] {
			continue
		}
		seen[off] = true
		n := block
		if off+n > size {
			n = size - off
		}
		if n <= 0 {
			continue
		}
		_, _ = io.WriteString(h, strconv.FormatInt(off, 10))
		_, _ = io.WriteString(h, "\x00")
		got, readErr := f.ReadAt(buf[:n], off)
		if readErr != nil && readErr != io.EOF {
			return "", readErr
		}
		if int64(got) != n {
			return "", io.ErrUnexpectedEOF
		}
		if _, err := h.Write(buf[:got]); err != nil {
			return "", err
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
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
	downloadHistoryStateV85.SaveMu.Lock()
	defer downloadHistoryStateV85.SaveMu.Unlock()

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

func loadSavedResultsForHistoryV85() map[int]Result {
	f, err := os.Open(savedResultsFileV85())
	if err != nil {
		return nil
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return nil
	}
	defer gz.Close()
	var rows []Result
	if json.NewDecoder(gz).Decode(&rows) != nil {
		return nil
	}
	out := make(map[int]Result, len(rows))
	for _, row := range rows {
		out[row.ID] = row
	}
	return out
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
	savedResults := loadSavedResultsForHistoryV85()
	updates := make([]downloadHistoryEntryV85, 0, len(jobs))
	for _, job := range jobs {
		if job.Status != "completed" || strings.TrimSpace(job.OutputPath) == "" {
			continue
		}
		fileInfo, statErr := os.Stat(job.OutputPath)
		if statErr != nil || fileInfo.IsDir() {
			continue
		}
		source := job.Source
		name := job.Name
		identityURL := job.URL
		size := job.BytesTotal
		if saved, ok := savedResults[job.ResultID]; ok {
			if s := strings.TrimSpace(saved.Remote.Source); s != "" {
				source = s
			}
			if n := strings.TrimSpace(saved.Remote.Name); n != "" {
				name = n
			}
			if stable := historyRemoteURLV85(saved); stable != "" {
				identityURL = stable
			}
			if saved.Remote.Size > 0 && !saved.Remote.ApproxSize {
				size = saved.Remote.Size
			}
		}
		if size <= 0 {
			size = job.BytesDone
		}
		if size <= 0 {
			size = fileInfo.Size()
		}
		key := downloadHistoryKeyV85(source, identityURL, name, size)
		finished := job.FinishedAt
		if finished <= 0 {
			finished = fileInfo.ModTime().Unix()
		}
		quickHash, _ := quickFileFingerprintV85(job.OutputPath, fileInfo.Size())
		updates = append(updates, downloadHistoryEntryV85{
			Key:        key,
			Source:     source,
			Name:       name,
			Bytes:      size,
			OutputPath: job.OutputPath,
			FinishedAt: finished,
			FileSize:   fileInfo.Size(),
			FileMTime:  fileInfo.ModTime().UnixNano(),
			QuickHash:  quickHash,
		})
	}
	changed := false
	downloadHistoryStateV85.Lock()
	for _, row := range updates {
		if old, ok := downloadHistoryStateV85.Entries[row.Key]; !ok || old.OutputPath != row.OutputPath || old.FileMTime != row.FileMTime || old.FileSize != row.FileSize || old.FinishedAt != row.FinishedAt || old.QuickHash != row.QuickHash {
			downloadHistoryStateV85.Entries[row.Key] = row
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
	source := strings.ToUpper(strings.TrimSpace(res.Remote.Source))
	if source == "MEGA" {
		if u := strings.TrimSpace(resultDownloadURL(res)); u != "" {
			return u
		}
	}
	// For extractor-based downloads use the original page/source identity, not
	// a temporary CDN URL from DirectURL. yt-dlp provider IDs are stable across
	// expiring signatures and title changes.
	if source == "YT-DLP" {
		base := strings.TrimSpace(res.Remote.URL)
		provider := strings.TrimSpace(res.Remote.ProviderID)
		extractor := strings.TrimSpace(res.Remote.Extractor)
		if provider != "" {
			return base + "\x1d" + extractor + "\x1d" + provider
		}
		if base != "" {
			return base
		}
	}
	if u := strings.TrimSpace(res.Remote.URL); u != "" {
		return u
	}
	if u := strings.TrimSpace(res.Remote.DirectURL); u != "" {
		return u
	}
	return strings.TrimSpace(resultDownloadURL(res))
}

func persistentDownloadHistoryDecisionV85(res Result) (DownloadGuardDecision, bool) {
	ensureDownloadHistoryWatcherV85()
	// Close the sub-500ms race between a just-completed queue save and an
	// immediate second preflight. The sync is cheap when queue mtime is unchanged.
	syncCompletedQueueToHistoryV85()
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
	if row.QuickHash != "" {
		currentHash, hashErr := quickFileFingerprintV85(row.OutputPath, st.Size())
		if hashErr != nil || currentHash != row.QuickHash {
			return DownloadGuardDecision{}, false
		}
	} else if row.FileMTime != 0 && st.ModTime().UnixNano() != row.FileMTime {
		// Compatibility with history rows created before content fingerprints.
		return DownloadGuardDecision{}, false
	}
	d := DownloadGuardDecision{
		ResultID:   res.ID,
		Name:       res.Remote.Name,
		Verdict:    guardDuplicate,
		Reason:     "Acest fișier apare în istoricul persistent al descărcărilor finalizate, iar amprenta conținutului local confirmă că fișierul rezultat există încă neschimbat.",
		LocalPath:  row.OutputPath,
		Method:     "download-history",
		Candidates: 1,
	}
	return decorateGuardDecision(d), true
}
