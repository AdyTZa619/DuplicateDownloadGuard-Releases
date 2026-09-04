package main

import (
	"compress/gzip"
	"encoding/gob"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type persistenceSpecV85 struct {
	Name     string
	Validate func(string) error
}

type persistenceStampV85 struct {
	Size  int64
	MTime int64
}

var persistenceRecoveryOnceV85 sync.Once

func validateJSONObjectFileV85(path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if len(b) == 0 || !json.Valid(b) {
		return errors.New("JSON invalid sau gol")
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	if raw == nil {
		return errors.New("JSON nu este obiect")
	}
	return nil
}

func validateIndexFileV85(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	var rows map[string]FileEntry
	if err := gob.NewDecoder(gz).Decode(&rows); err != nil {
		_ = gz.Close()
		return err
	}
	// Force gzip to EOF so a truncated stream/footer cannot be accepted merely
	// because gob finished decoding before the checksum was read.
	_, copyErr := io.Copy(io.Discard, gz)
	closeErr := gz.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

func validateResultsFileV85(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	dec := json.NewDecoder(gz)
	var rows []Result
	if err := dec.Decode(&rows); err != nil {
		_ = gz.Close()
		return err
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		_ = gz.Close()
		if err == nil {
			return errors.New("date suplimentare după JSON")
		}
		return err
	}
	_, copyErr := io.Copy(io.Discard, gz)
	closeErr := gz.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

func persistenceSpecsV85() []persistenceSpecV85 {
	return []persistenceSpecV85{
		{Name: "config.json", Validate: validateJSONObjectFileV85},
		{Name: "manual_decisions.json", Validate: validateJSONObjectFileV85},
		{Name: "index.gob.gz", Validate: validateIndexFileV85},
		{Name: "last_results.json.gz", Validate: validateResultsFileV85},
	}
}

func validPersistenceFileV85(path string, validate func(string) error) bool {
	st, err := os.Stat(path)
	return err == nil && !st.IsDir() && st.Size() > 0 && validate(path) == nil
}

func copyValidatedPersistenceV85(src, dst string, validate func(string) error) error {
	if !validPersistenceFileV85(src, validate) {
		return errors.New("sursa persistentă nu este validă")
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	tmp := dst + ".recovering"
	_ = os.Remove(tmp)
	if err := copyFileSimple(src, tmp); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := validate(tmp); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := replaceCacheFileV85(tmp, dst); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func persistenceFileNewerV85(candidate, primary string) bool {
	candidateInfo, err := os.Stat(candidate)
	if err != nil || candidateInfo.IsDir() {
		return false
	}
	primaryInfo, err := os.Stat(primary)
	if err != nil || primaryInfo.IsDir() {
		return true
	}
	return candidateInfo.ModTime().After(primaryInfo.ModTime())
}

func recoverPersistenceDirV85(dir string) []string {
	recovered := []string{}
	for _, spec := range persistenceSpecsV85() {
		primary := filepath.Join(dir, spec.Name)
		primaryValid := validPersistenceFileV85(primary, spec.Validate)
		pending := primary + ".tmp"

		// saveIndex/saveResults/saveDecisions write a fully closed .tmp before the
		// final rename. If Windows/process termination left that newer transaction
		// behind, it represents the latest committed state even when the old primary
		// is still perfectly readable. Promote only a fully validated, newer temp.
		if validPersistenceFileV85(pending, spec.Validate) && (!primaryValid || persistenceFileNewerV85(pending, primary)) {
			if copyValidatedPersistenceV85(pending, primary, spec.Validate) == nil {
				recovered = append(recovered, spec.Name)
				primaryValid = true
				_ = os.Remove(pending)
			}
		}
		if primaryValid {
			continue
		}

		backup := primary + ".good.bak"
		if validPersistenceFileV85(backup, spec.Validate) {
			if copyValidatedPersistenceV85(backup, primary, spec.Validate) == nil {
				recovered = append(recovered, spec.Name)
			}
		}
	}
	return recovered
}

func persistenceStampForV85(path string) (persistenceStampV85, bool) {
	st, err := os.Stat(path)
	if err != nil || st.IsDir() {
		return persistenceStampV85{}, false
	}
	return persistenceStampV85{Size: st.Size(), MTime: st.ModTime().UnixNano()}, true
}

func snapshotPersistenceDirV85(dir string, stamps map[string]persistenceStampV85) {
	for _, spec := range persistenceSpecsV85() {
		primary := filepath.Join(dir, spec.Name)
		stamp, ok := persistenceStampForV85(primary)
		if !ok {
			continue
		}
		if old, exists := stamps[spec.Name]; exists && old == stamp {
			continue
		}
		// Only a fully decodable file is allowed to replace the known-good copy.
		if !validPersistenceFileV85(primary, spec.Validate) {
			continue
		}
		backup := primary + ".good.bak"
		if err := copyValidatedPersistenceV85(primary, backup, spec.Validate); err == nil {
			stamps[spec.Name] = stamp
		}
	}
}

func startPersistenceRecoveryV85() {
	persistenceRecoveryOnceV85.Do(func() {
		// Unit-test binaries must not inspect or mutate the directory containing
		// the temporary test executable. Tests call the helpers with t.TempDir().
		base := strings.ToLower(filepath.Base(os.Args[0]))
		if strings.Contains(base, ".test") {
			return
		}
		dir := filepath.Join(executableDir(), "data")
		_ = os.MkdirAll(dir, 0755)
		recovered := recoverPersistenceDirV85(dir)
		if len(recovered) > 0 {
			// Logging is not initialized yet. Leave a tiny recovery marker that the
			// user/support can inspect without risking another dependency at startup.
			_ = os.WriteFile(filepath.Join(dir, "persistence_recovery.last.txt"), []byte(time.Now().Format(time.RFC3339)+"\n"+strings.Join(recovered, "\n")), 0644)
		}
		stamps := map[string]persistenceStampV85{}
		snapshotPersistenceDirV85(dir, stamps)
		go func() {
			ticker := time.NewTicker(3 * time.Second)
			defer ticker.Stop()
			for range ticker.C {
				snapshotPersistenceDirV85(dir, stamps)
			}
		}()
	})
}

// Run before main/newApp. This allows recovery of a broken primary file before
// loadConfig/loadIndex/loadResults/loadDecisions attempt to read it.
var persistenceRecoveryStartedV85 = func() bool {
	startPersistenceRecoveryV85()
	return true
}()
