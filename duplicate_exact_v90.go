package main

import (
	"context"
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"hash"
	"io"
	"os"
	"strings"
)

// Revalidate the open file before trusting a persistent digest. The live index
// can legitimately be seconds old when a download or another program changes
// the file. Hashing never holds App.mu while reading the disk.
func (a *App) ensureHashContextV90(ctx context.Context, path, kind string) (string, error) {
	kind = strings.ToLower(strings.ReplaceAll(kind, "-", ""))
	if kind != "sha256" && kind != "md5" {
		return "", errors.New("unsupported digest")
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	a.mu.RLock()
	e, ok := a.index[path]
	a.mu.RUnlock()
	if !ok {
		return "", os.ErrNotExist
	}
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	before, err := f.Stat()
	if err != nil {
		return "", err
	}
	if !before.Mode().IsRegular() {
		return "", errors.New("not a regular file")
	}
	identity := hashFileIdentityV90(f, before)
	if before.Size() == e.Size && before.ModTime().UnixNano() == e.MTime && identity != "" && identity == e.HashIdentity {
		if kind == "sha256" && e.SHA256 != "" {
			return e.SHA256, nil
		}
		if kind == "md5" && e.MD5 != "" {
			return e.MD5, nil
		}
	} else {
		e.SHA256, e.MD5 = "", ""
	}
	var h hash.Hash = sha256.New()
	if kind == "md5" {
		h = md5.New()
	}
	buf := make([]byte, 256<<10)
	for {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		n, readErr := f.Read(buf)
		if n > 0 {
			_, _ = h.Write(buf[:n])
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return "", readErr
		}
	}
	after, err := f.Stat()
	if err != nil {
		return "", err
	}
	current, err := os.Stat(path)
	if err != nil || !os.SameFile(before, current) || after.Size() != before.Size() ||
		after.ModTime() != before.ModTime() || hashFileIdentityV90(f, after) != identity {
		return "", errors.New("file changed while computing digest; retry verification")
	}
	digest := hex.EncodeToString(h.Sum(nil))
	e.Size, e.MTime, e.HashIdentity = after.Size(), after.ModTime().UnixNano(), identity
	if kind == "sha256" {
		e.SHA256 = digest
	} else {
		e.MD5 = digest
	}
	a.mu.Lock()
	if _, present := a.index[path]; present {
		a.index[path] = e
	}
	a.mu.Unlock()
	return digest, nil
}
