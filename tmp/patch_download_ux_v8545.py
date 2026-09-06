from pathlib import Path


def replace_once(text: str, old: str, new: str, label: str) -> str:
    if old not in text:
        raise SystemExit(f"marker not found: {label}")
    return text.replace(old, new, 1)


# 1) Reuse the live index that the source comparison has just built.
p = Path("download_guard.go")
s = p.read_text(encoding="utf-8")
s = replace_once(
    s,
    '''type DownloadGuardReport struct {\n\tMode         string                  `json:"mode"`\n\tDecisions    []DownloadGuardDecision `json:"decisions"`\n\tCounts       map[string]int          `json:"counts"`\n\tScannedFiles int                     `json:"scannedFiles"`\n\tScannedBytes int64                   `json:"scannedBytes"`\n\tScannedRoots []string                `json:"scannedRoots"`\n\tDurationMS   int64                   `json:"durationMs"`\n}''',
    '''type DownloadGuardReport struct {\n\tMode             string                  `json:"mode"`\n\tDecisions        []DownloadGuardDecision `json:"decisions"`\n\tCounts           map[string]int          `json:"counts"`\n\tScannedFiles     int                     `json:"scannedFiles"`\n\tScannedBytes     int64                   `json:"scannedBytes"`\n\tScannedRoots     []string                `json:"scannedRoots"`\n\tDurationMS       int64                   `json:"durationMs"`\n\tReusedFreshIndex bool                    `json:"reusedFreshIndex,omitempty"`\n\tIndexAgeMS       int64                   `json:"indexAgeMs,omitempty"`\n}''',
    "DownloadGuardReport",
)
s = replace_once(
    s,
    '''\tpaths = append(paths, configuredDownload, destination)\n\treturn compactGuardRoots(paths)''',
    '''\tpaths = append(paths, configuredDownload, destination)\n\tif strings.TrimSpace(configuredDownload) == "" {\n\t\tpaths = append(paths, portableDownloadsDir())\n\t}\n\treturn compactGuardRoots(paths)''',
    "guardRoots",
)
s = replace_once(
    s,
    '''\tout := make([]FileEntry, 0, len(entries))\n\tfor _, e := range entries {\n\t\tout = append(out, e)\n\t}\n\treturn out, scan, nil\n}''',
    '''\tout := make([]FileEntry, 0, len(entries))\n\tfor _, e := range entries {\n\t\tout = append(out, e)\n\t}\n\ta.rememberGuardRefreshV8545(out, scan)\n\treturn out, scan, nil\n}''',
    "refreshLiveIndexForGuard return",
)
s = replace_once(
    s,
    '''\tentries, scan, err := a.refreshLiveIndexForGuard(ctx, destination)\n\tif err != nil {\n\t\treturn report, err\n\t}\n\treport.ScannedFiles, report.ScannedBytes, report.ScannedRoots = scan.Files, scan.Bytes, scan.Roots''',
    '''\tentries, scan, indexAge, reusedFreshIndex := a.reuseFreshGuardIndexV8545(destination)\n\tvar err error\n\tif !reusedFreshIndex {\n\t\tentries, scan, err = a.refreshLiveIndexForGuard(ctx, destination)\n\t\tif err != nil {\n\t\t\treturn report, err\n\t\t}\n\t}\n\treport.ScannedFiles, report.ScannedBytes, report.ScannedRoots = scan.Files, scan.Bytes, scan.Roots\n\treport.ReusedFreshIndex = reusedFreshIndex\n\tif reusedFreshIndex {\n\t\treport.IndexAgeMS = indexAge.Milliseconds()\n\t}''',
    "runDownloadGuard refresh",
)
s = replace_once(
    s,
    '''\ta.logf("ExactGuard %s: %d download, %d duplicate blocate, %d review • scan live %d fișiere în %d ms", mode, report.Counts[guardDownload], report.Counts[guardDuplicate], report.Counts[guardReview], report.ScannedFiles, report.DurationMS)''',
    '''\tindexSource := "scan live"\n\tif report.ReusedFreshIndex {\n\t\tindexSource = fmt.Sprintf("index proaspăt reutilizat (%d ms)", report.IndexAgeMS)\n\t}\n\ta.logf("ExactGuard %s: %d download, %d duplicate blocate, %d review • %s • %d fișiere în %d ms", mode, report.Counts[guardDownload], report.Counts[guardDuplicate], report.Counts[guardReview], indexSource, report.ScannedFiles, report.DurationMS)''',
    "guard log",
)
p.write_text(s, encoding="utf-8")

Path("download_guard_fresh_cache.go").write_text(r'''package main

import (
	"sort"
	"strings"
	"sync"
	"time"
)

// The source comparison already performs a live HDD walk. A download started
// immediately afterwards can safely reuse that exact snapshot instead of
// walking the same roots again. The TTL is intentionally short.
const guardFreshIndexTTLV8545 = 30 * time.Second

type guardRefreshSnapshotV8545 struct {
	At       time.Time
	RootsKey string
	Scan     guardScan
	Entries  []FileEntry
}

var guardRefreshSnapshotsV8545 sync.Map

func guardRootsKeyV8545(roots []string) string {
	keys := make([]string, 0, len(roots))
	for _, root := range roots {
		keys = append(keys, pathKey(root))
	}
	sort.Strings(keys)
	return strings.Join(keys, "\n")
}

func (a *App) rememberGuardRefreshV8545(entries []FileEntry, scan guardScan) {
	copyEntries := append([]FileEntry(nil), entries...)
	copyScan := scan
	copyScan.Roots = append([]string(nil), scan.Roots...)
	guardRefreshSnapshotsV8545.Store(a, guardRefreshSnapshotV8545{
		At:       time.Now(),
		RootsKey: guardRootsKeyV8545(scan.Roots),
		Scan:     copyScan,
		Entries:  copyEntries,
	})
}

func (a *App) reuseFreshGuardIndexV8545(destination string) ([]FileEntry, guardScan, time.Duration, bool) {
	raw, ok := guardRefreshSnapshotsV8545.Load(a)
	if !ok {
		return nil, guardScan{}, 0, false
	}
	snapshot, ok := raw.(guardRefreshSnapshotV8545)
	if !ok {
		return nil, guardScan{}, 0, false
	}
	age := time.Since(snapshot.At)
	if age < 0 || age > guardFreshIndexTTLV8545 {
		return nil, guardScan{}, age, false
	}
	roots := a.guardRoots(destination)
	if guardRootsKeyV8545(roots) != snapshot.RootsKey {
		return nil, guardScan{}, age, false
	}
	entries := append([]FileEntry(nil), snapshot.Entries...)
	scan := snapshot.Scan
	scan.Roots = append([]string(nil), snapshot.Scan.Roots...)
	return entries, scan, age, true
}
''', encoding="utf-8")

Path("download_guard_fresh_cache_test.go").write_text(r'''package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func TestDownloadGuardReusesFreshCompareIndexV8545(t *testing.T) {
	collection, download := t.TempDir(), t.TempDir()
	data := []byte("fresh-index-content-v8545")
	local := filepath.Join(collection, "renamed-local.bin")
	if err := os.WriteFile(local, data, 0644); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	remote := RemoteItem{
		Name: "remote.bin", Size: int64(len(data)), Source: "HTTP",
		HashType: "sha256", Hash: hex.EncodeToString(sum[:]),
	}
	a := guardTestApp(t, collection, download, remote)
	a.cfg.LiveRefreshCompare = true
	a.compareRemote(context.Background(), []RemoteItem{remote}, "balanced")

	report, err := a.runDownloadGuard(context.Background(), a.results, download, guardModeSmart)
	if err != nil {
		t.Fatal(err)
	}
	if !report.ReusedFreshIndex {
		t.Fatalf("expected fresh index reuse: %#v", report)
	}
	if report.IndexAgeMS < 0 || report.IndexAgeMS > guardFreshIndexTTLV8545.Milliseconds() {
		t.Fatalf("unexpected index age: %dms", report.IndexAgeMS)
	}
	if got := report.Decisions[0]; got.Verdict != guardDuplicate || got.LocalPath != local {
		t.Fatalf("unexpected decision: %#v", got)
	}
}
''', encoding="utf-8")

# 2) Make FolderWatch output a real .crawljob and use stable GoFile links.
Path("jdownloader_bridge_v8545.go").write_text(r'''package main

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
''', encoding="utf-8")

p = Path("v7_extra.go")
s = p.read_text(encoding="utf-8")
old = '''\t\tu := resultDownloadURL(x)\n\t\tif u != "" && !seen[u] {\n\t\t\tseen[u] = true\n\t\t\tlines = append(lines, u)\n\t\t}\n\t}\n\tif len(lines) == 0 {\n\t\thttp.Error(w, "nu există linkuri exportabile", 400)'''
new = '''\t\tu := jdownloaderURLForResultV8545(x)\n\t\tif u != "" && !seen[u] {\n\t\t\tseen[u] = true\n\t\t\tlines = append(lines, u)\n\t\t}\n\t}\n\tif len(lines) == 0 {\n\t\thttp.Error(w, "nu există linkuri exportabile", 400)'''
s = replace_once(s, old, new, "JDownloader URL selection")
s = replace_once(
    s,
    '''\tif e := os.WriteFile(p, []byte(strings.Join(lines, "\\r\\n")+"\\r\\n"), 0644); e != nil {\n\t\thttp.Error(w, e.Error(), 500)\n\t\treturn\n\t}''',
    '''\tif e := writeJDownloaderCrawlJobV8545(p, lines, downloadDest); e != nil {\n\t\thttp.Error(w, e.Error(), 500)\n\t\treturn\n\t}''',
    "FolderWatch crawljob writer",
)
p.write_text(s, encoding="utf-8")

Path("jdownloader_bridge_v8545_test.go").write_text(r'''package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestJDownloaderGoFileUsesStablePerFileURLV8545(t *testing.T) {
	res := Result{Remote: RemoteItem{
		Source: "GOFILE", URL: "https://gofile.io/d/lNtYpg", ProviderID: "abc123",
		DirectURL: "https://store9.gofile.io/signed/temporary.mp4",
	}}
	got := jdownloaderURLForResultV8545(res)
	if got != "https://gofile.io/?c=lNtYpg#file=abc123" {
		t.Fatalf("unexpected GoFile JD URL: %s", got)
	}
}

func TestWriteJDownloaderCrawlJobIsValidJSONV8545(t *testing.T) {
	path := filepath.Join(t.TempDir(), "DDG.crawljob")
	if err := writeJDownloaderCrawlJobV8545(path, []string{"https://example.com/a", "https://example.com/b"}, `H:\\Downloads`); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var jobs []jdCrawlJobV8545
	if err := json.Unmarshal(b, &jobs); err != nil {
		t.Fatalf("invalid crawljob JSON: %v\n%s", err, b)
	}
	if len(jobs) != 2 || jobs[0].AutoStart != "FALSE" || jobs[0].AutoConfirm != "FALSE" || !strings.Contains(jobs[0].DownloadFolder, "Downloads") {
		t.Fatalf("unexpected crawljob: %#v", jobs)
	}
}
''', encoding="utf-8")

# 3) Expose the existing guard report UI to the focused download-actions module.
p = Path("web/exact_guard.js")
s = p.read_text(encoding="utf-8")
s = replace_once(
    s,
    '''    document.getElementById('guardModal').classList.remove('hidden');\n  }\n\n  window.closeGuardReport = function () {''',
    '''    document.getElementById('guardModal').classList.remove('hidden');\n  }\n\n  window.ddgShowGuardReportV8545 = showGuardReport;\n\n  window.closeGuardReport = function () {''',
    "export guard report",
)
p.write_text(s, encoding="utf-8")

# 4) Focused UI layer: one-click internal queue + selected-only JDownloader.
Path("web/download_actions_v8545.js").write_text(r'''(() => {
  'use strict';

  function counts(report) {
    const c = report?.counts || {};
    return {
      duplicate: Number(c.DUPLICATE || 0),
      review: Number(c.REVIEW || 0),
      download: Number(c.DOWNLOAD || 0)
    };
  }

  function queueNeedsAttention(data) {
    const c = counts(data?.guard);
    return c.duplicate > 0 || c.review > 0 || (Array.isArray(data?.rejected) && data.rejected.length > 0);
  }

  async function rowsForIDs(ids) {
    const wanted = new Set(ids.map(Number));
    const rows = [];
    let offset = 0;
    for (let page = 0; page < 100 && wanted.size; page++) {
      const data = await api(`/api/results?offset=${offset}&limit=1000&status=ALL`);
      const batch = Array.isArray(data.rows) ? data.rows : [];
      for (const row of batch) {
        if (wanted.has(Number(row.id))) {
          rows.push(row);
          wanted.delete(Number(row.id));
        }
      }
      if (!batch.length || offset + batch.length >= Number(data.total || 0)) break;
      offset += batch.length;
    }
    return rows;
  }

  function gofileJDURL(row) {
    const r = row?.remote || {};
    if (String(r.source || '').toUpperCase() !== 'GOFILE' || !r.providerId) return '';
    try {
      const u = new URL(r.url || '');
      const parts = u.pathname.split('/').filter(Boolean);
      if (parts.length >= 2 && parts[0].toLowerCase() === 'd' && parts[1]) {
        return `https://gofile.io/?c=${encodeURIComponent(parts[1])}#file=${encodeURIComponent(r.providerId)}`;
      }
    } catch (_) {}
    return '';
  }

  function jdURLForRow(row) {
    const r = row?.remote || {};
    return gofileJDURL(row) || String(r.directUrl || r.url || '').trim();
  }

  function checkJDownloader() {
    return new Promise(resolve => {
      document.getElementById('ddgJDCheckScript')?.remove();
      window.jdownloader = false;
      const script = document.createElement('script');
      script.id = 'ddgJDCheckScript';
      script.src = `http://127.0.0.1:9666/jdcheck.js?_ddg=${Date.now()}`;
      let finished = false;
      const finish = value => {
        if (finished) return;
        finished = true;
        script.remove();
        resolve(Boolean(value));
      };
      script.onload = () => finish(window.jdownloader === true);
      script.onerror = () => finish(false);
      document.head.appendChild(script);
      setTimeout(() => finish(window.jdownloader === true), 1200);
    });
  }

  function submitFlashGot(rows, destination) {
    const urls = [];
    const descriptions = [];
    const seen = new Set();
    for (const row of rows) {
      const url = jdURLForRow(row);
      if (!url || seen.has(url)) continue;
      seen.add(url);
      urls.push(url);
      descriptions.push(row?.remote?.name || row?.remote?.path || 'DDG');
    }
    if (!urls.length) throw new Error('Selecția nu conține linkuri compatibile cu JDownloader.');

    let iframe = document.getElementById('ddgJDTarget');
    if (!iframe) {
      iframe = document.createElement('iframe');
      iframe.id = 'ddgJDTarget';
      iframe.name = 'ddgJDTarget';
      iframe.style.display = 'none';
      document.body.appendChild(iframe);
    }
    const form = document.createElement('form');
    form.method = 'POST';
    form.action = 'http://127.0.0.1:9666/flashgot';
    form.target = 'ddgJDTarget';
    form.style.display = 'none';
    const add = (name, value) => {
      const input = document.createElement('input');
      input.type = 'hidden';
      input.name = name;
      input.value = value;
      form.appendChild(input);
    };
    add('urls', urls.join('\n'));
    add('description', descriptions.join('\n'));
    add('package', 'Duplicate Download Guard');
    if (destination) add('dir', destination);
    document.body.appendChild(form);
    form.submit();
    setTimeout(() => form.remove(), 1500);
    return urls.length;
  }

  async function folderWatchFallback(ids) {
    const folder = String(cfg?.jdFolder || '').trim();
    if (!folder) return false;
    const data = await api('/api/download/jd2', {
      method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ ids, folder })
    });
    if (window.ddgShowGuardReportV8545 && counts(data.guard).duplicate + counts(data.guard).review > 0) {
      window.ddgShowGuardReportV8545(data.guard, { ids, destination: cfg?.downloadDir || '', guardMode: cfg?.downloadGuardMode || 'smart' }, data.count || 0);
    }
    toast(`JDownloader FolderWatch: ${data.count || 0} fișier(e) selectate pregătite`);
    return true;
  }

  async function sendSelectedJDownloaderV8545() {
    const ids = typeof idsForAction === 'function' ? idsForAction() : [];
    if (!ids.length) return toast('Selectează fișiere');
    const button = document.getElementById('jdSelectedBtn');
    if (button) {
      button.disabled = true;
      button.textContent = '⏳ JDownloader…';
    }
    try {
      const running = await checkJDownloader();
      if (!running) {
        if (await folderWatchFallback(ids)) return;
        throw new Error('JDownloader nu răspunde pe 127.0.0.1:9666. Pornește JDownloader sau setează FolderWatch în Reguli.');
      }

      const destination = document.getElementById('downloadDir')?.value?.trim() || cfg?.downloadDir || '';
      const mode = document.getElementById('downloadGuardMode')?.value || cfg?.downloadGuardMode || 'smart';
      const guard = await api('/api/download/preflight', {
        method: 'POST', headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ ids, destination, mode })
      });
      const allowed = (guard.decisions || []).filter(x => x.verdict === 'DOWNLOAD').map(x => Number(x.resultId));
      if (!allowed.length) {
        window.ddgShowGuardReportV8545?.(guard, { ids, destination, guardMode: mode }, 0);
        return toast('Nimic de trimis: selecția este duplicat sau necesită verificare');
      }
      const rows = await rowsForIDs(allowed);
      const count = submitFlashGot(rows, destination);
      const c = counts(guard);
      if (c.duplicate > 0 || c.review > 0) {
        window.ddgShowGuardReportV8545?.(guard, { ids, destination, guardMode: mode }, count);
      }
      toast(`Trimis în JDownloader LinkGrabber: ${count} fișier(e)`);
    } catch (error) {
      toast(error.message || String(error));
    } finally {
      if (button) {
        button.disabled = false;
        button.textContent = '↗ JDownloader';
      }
    }
  }

  async function downloadSelectedV8545() {
    const ids = typeof idsForAction === 'function' ? idsForAction() : [];
    if (!ids.length) return toast('Selectează fișiere');
    const destination = document.getElementById('downloadDir')?.value?.trim() || cfg?.downloadDir || '';
    const engine = document.getElementById('downloadMethod')?.value || cfg?.downloadMethod || 'auto';
    const mode = document.getElementById('downloadGuardMode')?.value || cfg?.downloadGuardMode || 'smart';
    const request = { ids, engine, destination, guardMode: mode };
    const button = document.getElementById('downloadGuardBtn');
    if (button) {
      button.disabled = true;
      button.textContent = '⏳ Pregătesc…';
    }
    try {
      const data = await api('/api/queue/add', {
        method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(request)
      });
      await loadResults();
      if (queueNeedsAttention(data)) {
        window.ddgShowGuardReportV8545?.(data.guard, request, data.added);
      } else {
        const modal = document.getElementById('guardModal');
        if (modal && !modal.classList.contains('hidden')) window.closeGuardReport?.();
      }
      await loadQueue();
      if (data.added > 0) {
        const suffix = data.guard?.reusedFreshIndex ? ' • fără a rescana HDD-ul' : '';
        toast(`${data.added} fișier(e) în coadă${suffix}`);
        if (!queueNeedsAttention(data)) goTab('downloads');
      } else if (!queueNeedsAttention(data)) {
        toast('Niciun fișier nou nu a fost adăugat în coadă');
      }
    } catch (error) {
      toast(error.message || String(error));
    } finally {
      if (button) {
        button.disabled = false;
        button.textContent = '⬇ Descarcă';
      }
    }
  }

  function installButtons() {
    const download = document.getElementById('downloadGuardBtn') || document.querySelector('button[onclick="downloadSelected()"]');
    if (!download) return false;
    download.textContent = '⬇ Descarcă';
    download.title = 'Un click: DDG verifică în fundal și pornește coada. Indexul tocmai actualizat este reutilizat, nu scanat încă o dată.';

    if (!document.getElementById('jdSelectedBtn')) {
      const jd = document.createElement('button');
      jd.className = 'btn';
      jd.id = 'jdSelectedBtn';
      jd.type = 'button';
      jd.textContent = '↗ JDownloader';
      jd.title = 'Trimite numai selecția confirmată ca lipsă în JDownloader LinkGrabber.';
      jd.addEventListener('click', sendSelectedJDownloaderV8545);
      download.insertAdjacentElement('afterend', jd);
    }
    window.downloadSelected = downloadSelectedV8545;
    window.sendSelectedJD2 = sendSelectedJDownloaderV8545;
    return true;
  }

  function install() {
    let tries = 0;
    const timer = setInterval(() => {
      tries++;
      if (installButtons() || tries >= 80) clearInterval(timer);
    }, 100);
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', () => setTimeout(install, 0), { once: true });
  } else {
    install();
  }
  window.ddgDownloadActionsV8545 = { install, sendSelectedJDownloaderV8545 };
})();
''', encoding="utf-8")

p = Path("web/preview_quick_v86.js")
s = p.read_text(encoding="utf-8")
s = replace_once(
    s,
    "    ['/provider_sources.js', 'ddgProviderSources'],\n    ['/updater_resilience.js', 'ddgUpdaterResilience']",
    "    ['/provider_sources.js', 'ddgProviderSources'],\n    ['/updater_resilience.js', 'ddgUpdaterResilience'],\n    ['/download_actions_v8545.js', 'ddgDownloadActionsV8545Script']",
    "download actions bootstrap",
)
p.write_text(s, encoding="utf-8")
