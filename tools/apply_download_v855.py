from pathlib import Path


def replace_once(text, old, new, label):
    count = text.count(old)
    if count != 1:
        raise SystemExit(f'{label}: expected 1 match, found {count}')
    return text.replace(old, new, 1)

# --- download_core_v855.go: legacy compatibility + final safeguards ---
p = Path('download_core_v855.go')
s = p.read_text(encoding='utf-8')
old = '''func sameQueueRemoteV855(j *DownloadJob, res Result) bool {\n\tif j == nil {\n\t\treturn false\n\t}\n\ta := stableRemoteKeyV855(remoteSnapshotFromJobV855(j))\n\tb := stableRemoteKeyV855(res.Remote)\n\treturn a != "" && b != "" && a == b\n}\n\nfunc (a *App) downloadResultForJobV855(j *DownloadJob) Result {\n'''
new = '''func jobHasRemoteSnapshotV855(j *DownloadJob) bool {\n\tif j == nil {\n\t\treturn false\n\t}\n\tr := j.Remote\n\treturn r.Name != "" || r.Path != "" || r.URL != "" || r.DirectURL != "" || r.Handle != "" || r.ProviderID != ""\n}\n\nfunc legacyQueueMatchesResultV855(j *DownloadJob, res Result) bool {\n\tif j == nil || jobHasRemoteSnapshotV855(j) {\n\t\treturn false\n\t}\n\tif !strings.EqualFold(strings.TrimSpace(j.Source), strings.TrimSpace(res.Remote.Source)) {\n\t\treturn false\n\t}\n\tif j.Name != "" && res.Remote.Name != "" && !strings.EqualFold(strings.TrimSpace(j.Name), strings.TrimSpace(res.Remote.Name)) {\n\t\treturn false\n\t}\n\tlegacyURL := strings.TrimSpace(j.URL)\n\tcurrentURL := strings.TrimSpace(resultDownloadURL(res))\n\treturn legacyURL != "" && currentURL != "" && legacyURL == currentURL\n}\n\nfunc sameQueueRemoteV855(j *DownloadJob, res Result) bool {\n\tif j == nil {\n\t\treturn false\n\t}\n\tif jobHasRemoteSnapshotV855(j) {\n\t\ta := stableRemoteKeyV855(j.Remote)\n\t\tb := stableRemoteKeyV855(res.Remote)\n\t\treturn a != "" && b != "" && a == b\n\t}\n\treturn legacyQueueMatchesResultV855(j, res)\n}\n\nfunc (a *App) downloadResultForJobV855(j *DownloadJob) Result {\n'''
s = replace_once(s, old, new, 'legacy identity helpers')

old = '''func isManifestRemoteV855(r RemoteItem) bool {\n'''
new = '''func ytDlpInputURLV855(res Result) string {\n\tif strings.EqualFold(res.Remote.Source, "YT-DLP") && strings.TrimSpace(res.Remote.URL) != "" {\n\t\treturn strings.TrimSpace(res.Remote.URL)\n\t}\n\tif strings.TrimSpace(res.Remote.DirectURL) != "" {\n\t\treturn strings.TrimSpace(res.Remote.DirectURL)\n\t}\n\treturn strings.TrimSpace(res.Remote.URL)\n}\n\nfunc isManifestRemoteV855(r RemoteItem) bool {\n'''
s = replace_once(s, old, new, 'yt-dlp input helper')

old = '''\tcase "yt-dlp":\n\t\tif a.detectYtDlp() == "" {\n\t\t\treturn errors.New("yt-dlp lipsește; instalează-l din AI & Tool Manager")\n\t\t}\n'''
new = '''\tcase "yt-dlp":\n\t\tif a.detectYtDlp() == "" {\n\t\t\treturn errors.New("yt-dlp lipsește; instalează-l din AI & Tool Manager")\n\t\t}\n\t\tif ytDlpInputURLV855(res) == "" {\n\t\t\treturn errors.New("yt-dlp nu are URL-ul paginii/streamului de descărcat")\n\t\t}\n'''
s = replace_once(s, old, new, 'yt-dlp validation')

old = '''\tpart := sourcePartPathV855(dest, res.Remote.Name, u)\n\tvar start int64\n\tif st, err := os.Stat(part); err == nil && !st.IsDir() {\n\t\tstart = st.Size()\n\t}\n\n\trequest := func(offset int64) (*http.Response, error) {\n'''
new = '''\tpart := sourcePartPathV855(dest, res.Remote.Name, u)\n\tvar start int64\n\tif st, err := os.Stat(part); err == nil && !st.IsDir() {\n\t\tstart = st.Size()\n\t}\n\tif start > 0 && res.Remote.Size > 0 && !res.Remote.ApproxSize && start == res.Remote.Size {\n\t\tif err := verifyDownloadedAgainstRemote(part, res.Remote); err == nil {\n\t\t\tfinal := collisionFreeFinalV855(dest, res.Remote.Name)\n\t\t\tif err := os.Rename(part, final); err == nil {\n\t\t\t\tprogress(start, start)\n\t\t\t\treturn final, nil\n\t\t\t}\n\t\t} else {\n\t\t\t_ = os.Remove(part)\n\t\t\tstart = 0\n\t\t}\n\t}\n\n\trequest := func(offset int64) (*http.Response, error) {\n'''
s = replace_once(s, old, new, 'completed part recovery')

old = '''\tdefer resp.Body.Close()\n\tif resp.StatusCode >= 400 {\n\t\tif resp.StatusCode == http.StatusForbidden && downloadRefererV855(res) != "" {\n\t\t\treturn "", errors.New("HTTP 403: serverul a refuzat downloadul chiar și cu Referer-ul paginii sursă; pot fi necesare cookies/autentificare")\n\t\t}\n\t\treturn "", fmt.Errorf("HTTP %d", resp.StatusCode)\n\t}\n\n\tflags := os.O_CREATE | os.O_WRONLY\n'''
new = '''\tdefer resp.Body.Close()\n\tif resp.StatusCode >= 400 {\n\t\tif resp.StatusCode == http.StatusForbidden && downloadRefererV855(res) != "" {\n\t\t\treturn "", errors.New("HTTP 403: serverul a refuzat downloadul chiar și cu Referer-ul paginii sursă; pot fi necesare cookies/autentificare")\n\t\t}\n\t\treturn "", fmt.Errorf("HTTP %d", resp.StatusCode)\n\t}\n\tcontentType := strings.ToLower(strings.TrimSpace(strings.Split(resp.Header.Get("Content-Type"), ";")[0]))\n\tif strings.HasPrefix(contentType, "text/html") && !strings.HasPrefix(strings.ToLower(res.Remote.ContentType), "text/html") {\n\t\treturn "", errors.New("serverul a returnat o pagină HTML în locul fișierului media; URL-ul direct poate fi expirat sau poate necesita autentificare")\n\t}\n\n\tflags := os.O_CREATE | os.O_WRONLY\n'''
s = replace_once(s, old, new, 'reject HTML response')

old = '''\tcase strings.Contains(msg, "http 404"):\n\t\treturn "HTTP_404", "Fișier indisponibil", "Rescanează sursa; URL-ul direct poate fi expirat."\n\tcase strings.Contains(msg, "url direct lipsă") || strings.Contains(msg, "url direct utilizabil"):\n'''
new = '''\tcase strings.Contains(msg, "http 404"):\n\t\treturn "HTTP_404", "Fișier indisponibil", "Rescanează sursa; URL-ul direct poate fi expirat."\n\tcase strings.Contains(msg, "pagină html"):\n\t\treturn "SOURCE_RESPONSE_HTML", "Sursa nu mai livrează fișierul", "Rescanează pagina/sursa; URL-ul direct poate fi expirat sau poate necesita autentificare."\n\tcase strings.Contains(msg, "url direct lipsă") || strings.Contains(msg, "url direct utilizabil") || strings.Contains(msg, "nu are url-ul"):\n'''
s = replace_once(s, old, new, 'HTML error classification')

p.write_text(s, encoding='utf-8', newline='\n')

# --- v8_extra.go: autonomous queue, deterministic engine, clear rejection ---
p = Path('v8_extra.go')
s = p.read_text(encoding='utf-8')
old = '''\tURL           string `json:"url,omitempty"`\n\tDestination   string `json:"destination"`\n\tEngine        string `json:"engine"`\n'''
new = '''\tURL             string     `json:"url,omitempty"`\n\tRemote          RemoteItem `json:"remote,omitempty"`\n\tRequestedEngine string     `json:"requestedEngine,omitempty"`\n\tDestination     string     `json:"destination"`\n\tEngine          string     `json:"engine"`\n'''
s = replace_once(s, old, new, 'DownloadJob snapshot fields')

start = s.index('func chooseQueueEngine(a *App, res Result, requested string) string {')
end = s.index('\n}\n\nfunc (q *DownloadQueue) runJob', start) + 3
old = s[start:end]
new = '''func chooseQueueEngine(a *App, res Result, requested string) string {\n\treturn chooseQueueEngineV855(res, requested)\n}\n'''
s = s[:start] + new + s[end:]

old = '''\tq.mu.Lock()\n\tj := q.findLocked(id)\n\tif j == nil {\n\t\tq.mu.Unlock()\n\t\treturn\n\t}\n\trid, engine, dest := j.ResultID, j.Engine, j.Destination\n\tq.mu.Unlock()\n\tres, ok := a.resultByID(rid)\n\tif !ok {\n\t\tq.update(a, id, func(x *DownloadJob) { x.Status = "failed"; x.Error = "rezultatul sursă nu mai există" })\n\t\treturn\n\t}\n\tq.mu.Lock()\n\tguardVersion, guardOverride, guardMode := 0, false, ""\n\tif current := q.findLocked(id); current != nil {\n\t\tguardVersion, guardOverride, guardMode = current.GuardVersion, current.GuardOverride, current.GuardMode\n\t}\n\tq.mu.Unlock()\n'''
new = '''\tq.mu.Lock()\n\tcurrent := q.findLocked(id)\n\tif current == nil {\n\t\tq.mu.Unlock()\n\t\treturn\n\t}\n\tsnapshot := *current\n\trid, engine, requestedEngine, dest := snapshot.ResultID, snapshot.Engine, snapshot.RequestedEngine, snapshot.Destination\n\tguardVersion, guardOverride, guardMode := snapshot.GuardVersion, snapshot.GuardOverride, snapshot.GuardMode\n\tq.mu.Unlock()\n\tres := a.downloadResultForJobV855(&snapshot)\n\tif stableRemoteKeyV855(res.Remote) == "" {\n\t\tq.update(a, id, func(x *DownloadJob) {\n\t\t\tx.Status = "failed"\n\t\t\tx.ErrorCode = "SOURCE_IDENTITY_MISSING"\n\t\t\tx.ErrorTitle = "Sursa jobului nu mai poate fi identificată"\n\t\t\tx.ErrorAction = "Elimină jobul și adaugă din nou fișierul din rezultatele scanării."\n\t\t\tx.Error = "Jobul vechi nu conține suficiente date despre sursa remote pentru un download sigur."\n\t\t\tx.Stage = "oprit înainte de transfer"\n\t\t\tx.FinishedAt = time.Now().Unix()\n\t\t})\n\t\treturn\n\t}\n'''
s = replace_once(s, old, new, 'runJob snapshot bootstrap')

old = '''\tif guardVersion != downloadGuardVersion {\n\t\treport, guardErr := a.runDownloadGuard(ctx, []Result{res}, dest, guardMode)\n'''
new = '''\tif guardVersion != downloadGuardVersion {\n\t\tguardRes := res\n\t\tguardRes.ID = -1 // detached queue verification must never mutate a reused live ResultID\n\t\treport, guardErr := a.runDownloadGuard(ctx, []Result{guardRes}, dest, guardMode)\n'''
s = replace_once(s, old, new, 'detached guard revalidation')

old = '''\t\tif refreshed, exists := a.resultByID(rid); exists {\n\t\t\tres = refreshed\n\t\t}\n\t}\n\tengine = chooseQueueEngine(a, res, engine)\n\tq.update(a, id, func(x *DownloadJob) {\n'''
new = '''\t\tif refreshed, exists := a.resultByID(rid); exists && sameQueueRemoteV855(&snapshot, refreshed) {\n\t\t\tres = refreshed\n\t\t}\n\t}\n\tengineRequest := requestedEngine\n\tif strings.TrimSpace(engineRequest) == "" {\n\t\tengineRequest = engine // legacy jobs keep their previously selected engine\n\t}\n\tengine = chooseQueueEngineV855(res, engineRequest)\n\tif validationErr := a.validateDownloadEngineV855(res, engine); validationErr != nil {\n\t\tcode, title, action := classifyDownloadErrorV855(engine, validationErr)\n\t\tq.update(a, id, func(x *DownloadJob) {\n\t\t\tx.Status = "paused"\n\t\t\tx.Engine = engine\n\t\t\tx.Error = validationErr.Error()\n\t\t\tx.ErrorCode, x.ErrorTitle, x.ErrorAction = code, title, action\n\t\t\tx.Stage = "downloadul nu a pornit"\n\t\t\tx.FinishedAt = time.Now().Unix()\n\t\t})\n\t\treturn\n\t}\n\tq.update(a, id, func(x *DownloadJob) {\n'''
s = replace_once(s, old, new, 'engine selection and validation')

old = '''\t\tcase "mega":\n\t\t\tq.update(a, id, func(x *DownloadJob) { x.Stage = "MEGAcmd așteaptă sesiunea / descarcă fișierul" })\n\t\t\t// Do not burn retry attempts merely because a scan is active. The\n\t\t\t// cancellable MEGA session gate waits safely until scan/preview releases\n\t\t\t// the single MEGAcmd session.\n\t\t\tmegaQueueMu.Lock()\n\t\t\terr = a.downloadMegaResults(ctx, []Result{res}, dest)\n'''
new = '''\t\tcase "mega":\n\t\t\tq.update(a, id, func(x *DownloadJob) { x.Stage = "MEGAcmd: pregătesc sesiunea și transferul" })\n\t\t\t// Keep queue execution detached from the mutable results table. The\n\t\t\t// MEGA downloader still updates the local index, but ID=-1 prevents it\n\t\t\t// from marking an unrelated row if IDs were reused by a later scan.\n\t\t\tmegaRes := res\n\t\t\tmegaRes.ID = -1\n\t\t\tmegaQueueMu.Lock()\n\t\t\terr = a.downloadMegaResults(ctx, []Result{megaRes}, dest)\n'''
s = replace_once(s, old, new, 'MEGA detached transfer')

old = '''\t\tcase "yt-dlp":\n\t\t\texe := a.detectYtDlp()\n\t\t\tif exe == "" {\n\t\t\t\terr = errors.New("yt-dlp lipsește")\n\t\t\t} else {\n\t\t\t\tpath, err = a.runYtDlpDownload(ctx, exe, res.Remote.URL, dest)\n\t\t\t}\n'''
new = '''\t\tcase "yt-dlp":\n\t\t\tq.update(a, id, func(x *DownloadJob) { x.Stage = "yt-dlp: extrag și descarc fluxul media" })\n\t\t\texe := a.detectYtDlp()\n\t\t\tif exe == "" {\n\t\t\t\terr = errors.New("yt-dlp lipsește")\n\t\t\t} else {\n\t\t\t\tpath, err = a.runYtDlpDownload(ctx, exe, ytDlpInputURLV855(res), dest)\n\t\t\t}\n'''
s = replace_once(s, old, new, 'yt-dlp input and stage')

old = '''\t\tdefault:\n\t\t\tu := resultDownloadURL(res)\n\t\t\tif u == "" {\n\t\t\t\terr = errors.New("URL direct lipsă")\n\t\t\t} else {\n\t\t\t\tpath, err = internalDownload(ctx, u, dest, res.Remote.Name, progress)\n\t\t\t}\n\t\t}\n'''
new = '''\t\tdefault:\n\t\t\tq.update(a, id, func(x *DownloadJob) { x.Stage = "HTTP: descarc direct cu resume" })\n\t\t\tpath, err = internalDownloadV855(ctx, res, dest, progress)\n\t\t}\n'''
s = replace_once(s, old, new, 'internal downloader switch')

old = '''\t\t\t} else {\n\t\t\t\ta.markDownloaded(res.ID, path)\n\t\t\t\tst, _ := os.Stat(path)\n'''
new = '''\t\t\t} else {\n\t\t\t\ta.markDownloadedResultV855(res, path)\n\t\t\t\tst, _ := os.Stat(path)\n'''
s = replace_once(s, old, new, 'safe result marking')

old = '''\t\tif err == nil {\n\t\t\terr = errors.New("motorul de download nu a returnat nici fișier, nici eroare")\n\t\t}\n\t\tif attempts > cfgRetries {\n'''
new = '''\t\tif err == nil {\n\t\t\terr = errors.New("motorul de download nu a returnat nici fișier, nici eroare")\n\t\t}\n\t\tif engine != "mega" && err != nil {\n\t\t\tcode, title, action := classifyDownloadErrorV855(engine, err)\n\t\t\tq.update(a, id, func(x *DownloadJob) {\n\t\t\t\tx.Error = err.Error()\n\t\t\t\tx.ErrorCode, x.ErrorTitle, x.ErrorAction = code, title, action\n\t\t\t\tx.Stage = "motorul a raportat o eroare"\n\t\t\t})\n\t\t\tif code == "TOOL_MISSING" || code == "ENGINE_INCOMPATIBLE" || code == "HTTP_403" || code == "SOURCE_URL_MISSING" || code == "SOURCE_RESPONSE_HTML" {\n\t\t\t\tq.update(a, id, func(x *DownloadJob) {\n\t\t\t\t\tx.Status = "paused"\n\t\t\t\t\tx.FinishedAt = time.Now().Unix()\n\t\t\t\t\tx.SpeedBps, x.ETA = 0, 0\n\t\t\t\t})\n\t\t\t\treturn\n\t\t\t}\n\t\t\tif code == "HTTP_404" {\n\t\t\t\tq.update(a, id, func(x *DownloadJob) {\n\t\t\t\t\tx.Status = "failed"\n\t\t\t\t\tx.FinishedAt = time.Now().Unix()\n\t\t\t\t\tx.SpeedBps, x.ETA = 0, 0\n\t\t\t\t})\n\t\t\t\treturn\n\t\t\t}\n\t\t}\n\t\tif attempts > cfgRetries {\n'''
s = replace_once(s, old, new, 'generic error classification')

old = '''\tq := queueFor(a)\n\tadded := 0\n\tariaRemove := []string{}\n\tq.mu.Lock()\n'''
new = '''\trequestedEngine := strings.ToLower(strings.TrimSpace(req.Engine))\n\tif requestedEngine == "" {\n\t\trequestedEngine = "auto"\n\t}\n\tselectedByID := map[int]Result{}\n\tengineByID := map[int]string{}\n\trejected := []map[string]any{}\n\tfor _, res := range selectedRows {\n\t\tselectedByID[res.ID] = res\n\t\tif !wanted[res.ID] {\n\t\t\tcontinue\n\t\t}\n\t\tengine := chooseQueueEngineV855(res, requestedEngine)\n\t\tif engineErr := a.validateDownloadEngineV855(res, engine); engineErr != nil {\n\t\t\tcode, title, action := classifyDownloadErrorV855(engine, engineErr)\n\t\t\trejected = append(rejected, map[string]any{"resultId": res.ID, "name": res.Remote.Name, "engine": engine, "code": code, "title": title, "message": engineErr.Error(), "action": action})\n\t\t\tdelete(wanted, res.ID)\n\t\t\tcontinue\n\t\t}\n\t\tengineByID[res.ID] = engine\n\t}\n\tq := queueFor(a)\n\tadded := 0\n\tariaRemove := []string{}\n\tq.mu.Lock()\n'''
s = replace_once(s, old, new, 'queue prevalidation')

old = '''\tfor _, job := range q.Jobs {\n\t\tdecision, exists := decisions[job.ResultID]\n\t\tif !exists || job.Status == "completed" || job.Status == "cancelled" || job.Status == "blocked" {\n\t\t\tcontinue\n\t\t}\n'''
new = '''\tfor _, job := range q.Jobs {\n\t\tdecision, exists := decisions[job.ResultID]\n\t\tselected, selectedExists := selectedByID[job.ResultID]\n\t\tif !exists || !selectedExists || !sameQueueRemoteV855(job, selected) || job.Status == "completed" || job.Status == "cancelled" || job.Status == "blocked" {\n\t\t\tcontinue\n\t\t}\n'''
s = replace_once(s, old, new, 'stable old-job guard mapping')

old = '''\tfor _, res := range rows {\n\t\tif !wanted[res.ID] {\n'''
new = '''\tfor _, res := range selectedRows {\n\t\tif !wanted[res.ID] {\n'''
s = replace_once(s, old, new, 'selectedRows queue creation')

old = '''\t\tfor _, j := range q.Jobs {\n\t\t\tif j.ResultID == res.ID && (j.Status == "queued" || j.Status == "running" || j.Status == "paused") {\n\t\t\t\tdup = true\n'''
new = '''\t\tfor _, j := range q.Jobs {\n\t\t\tif sameQueueRemoteV855(j, res) && (j.Status == "queued" || j.Status == "running" || j.Status == "paused") {\n\t\t\t\tdup = true\n'''
s = replace_once(s, old, new, 'stable duplicate job detection')

old = '''\t\tq.Jobs = append(q.Jobs, &DownloadJob{ID: jid, ResultID: res.ID, Name: res.Remote.Name, Source: res.Remote.Source, URL: resultDownloadURL(res), Destination: dest, Engine: chooseQueueEngine(a, res, req.Engine), Status: "queued", Priority: 0, BytesTotal: res.Remote.Size, MaxRetries: retries, GuardMode: report.Mode, GuardVerdict: decision.Verdict, GuardReason: decision.Reason, GuardMethod: decision.Method, GuardVersion: downloadGuardVersion, GuardAt: now, GuardOverride: decision.Verdict == guardReview && req.AllowReview, AddedAt: now, UpdatedAt: now})\n'''
new = '''\t\tq.Jobs = append(q.Jobs, &DownloadJob{ID: jid, ResultID: res.ID, Name: res.Remote.Name, Source: res.Remote.Source, URL: resultDownloadURL(res), Remote: res.Remote, RequestedEngine: requestedEngine, Destination: dest, Engine: engineByID[res.ID], Status: "queued", Priority: 0, BytesTotal: res.Remote.Size, MaxRetries: retries, GuardMode: report.Mode, GuardVerdict: decision.Verdict, GuardReason: decision.Reason, GuardMethod: decision.Method, GuardVersion: downloadGuardVersion, GuardAt: now, GuardOverride: decision.Verdict == guardReview && req.AllowReview, AddedAt: now, UpdatedAt: now})\n'''
s = replace_once(s, old, new, 'persist remote snapshot')

old = '''\tmessage := fmt.Sprintf("%d adăugate în coadă • %d duplicate blocate • %d necesită review", added, duplicates, review)\n\tjsonOut(w, map[string]any{"ok": true, "added": added, "destination": dest, "guard": report, "reviewOverride": req.AllowReview, "message": message})\n'''
new = '''\tmessage := fmt.Sprintf("%d adăugate în coadă • %d duplicate blocate • %d necesită review", added, duplicates, review)\n\tif len(rejected) > 0 {\n\t\tmessage += fmt.Sprintf(" • %d nu au pornit (configurație/sursă)", len(rejected))\n\t}\n\tjsonOut(w, map[string]any{"ok": true, "added": added, "rejected": rejected, "destination": dest, "guard": report, "reviewOverride": req.AllowReview, "message": message})\n'''
s = replace_once(s, old, new, 'queue response with rejections')

old = '''\tjsonOut(w, map[string]any{"jobs": rows, "summary": queueSummary(rows), "megaStatus": megaStatus, "downloadDir": func() string { a.mu.RLock(); defer a.mu.RUnlock(); return a.cfg.DownloadDir }()})\n'''
new = '''\tdownloadDir := func() string {\n\t\ta.mu.RLock()\n\t\tdefer a.mu.RUnlock()\n\t\tif strings.TrimSpace(a.cfg.DownloadDir) != "" {\n\t\t\treturn a.cfg.DownloadDir\n\t\t}\n\t\treturn portableDownloadsDir()\n\t}()\n\tjsonOut(w, map[string]any{"jobs": rows, "summary": queueSummary(rows), "megaStatus": megaStatus, "downloadDir": downloadDir})\n'''
s = replace_once(s, old, new, 'actual download folder in queue')

p.write_text(s, encoding='utf-8', newline='\n')

# --- aria2_rpc.go: preserve Referer when user explicitly chooses aria2 ---
p = Path('aria2_rpc.go')
s = p.read_text(encoding='utf-8')
old = '''\topt := map[string]string{"dir": dest, "out": sanitizeFilename(res.Remote.Name), "continue": "true", "auto-file-renaming": "false", "allow-overwrite": "false", "file-allocation": "none", "split": strconv.Itoa(conn), "max-connection-per-server": strconv.Itoa(conn), "min-split-size": "1M", "max-tries": strconv.Itoa(retries + 1), "retry-wait": "2"}\n\tif limit > 0 {\n'''
new = '''\topt := map[string]string{"dir": dest, "out": sanitizeFilename(res.Remote.Name), "continue": "true", "auto-file-renaming": "false", "allow-overwrite": "false", "file-allocation": "none", "split": strconv.Itoa(conn), "max-connection-per-server": strconv.Itoa(conn), "min-split-size": "1M", "max-tries": strconv.Itoa(retries + 1), "retry-wait": "2"}\n\tif referer := downloadRefererV855(res); referer != "" {\n\t\topt["referer"] = referer\n\t}\n\tif limit > 0 {\n'''
s = replace_once(s, old, new, 'aria2 Referer')
p.write_text(s, encoding='utf-8', newline='\n')

# --- web/exact_guard.js: simple download UX + current controls + explicit rejects ---
p = Path('web/exact_guard.js')
s = p.read_text(encoding='utf-8')
s = replace_once(s, "downloadButton.textContent = '🛡 Verifică inteligent + descarcă';", "downloadButton.textContent = '⬇ Descarcă selectate';", 'initial button label')
s = replace_once(s, "downloadButton.title = 'Rescanează HDD-urile, verifică istoricul, hash-ul și variantele media înainte de orice download';", "downloadButton.title = 'Smart Guard verifică automat duplicatele înainte de transfer; apoi pornește downloadul sau explică exact de ce nu poate porni.';", 'button title')
old = '''      const destination = cfg.downloadDir || document.getElementById('downloadDir')?.value || '';\n      if (!destination) return toast('Setează folderul de download');\n      const mode = document.getElementById('downloadGuardMode')?.value || cfg.downloadGuardMode || 'smart';\n      const request = { ids, engine: cfg.downloadMethod || 'auto', destination, guardMode: mode };\n'''
new = '''      const destination = document.getElementById('downloadDir')?.value?.trim() || cfg.downloadDir || '';\n      const engine = document.getElementById('downloadMethod')?.value || cfg.downloadMethod || 'auto';\n      const mode = document.getElementById('downloadGuardMode')?.value || cfg.downloadGuardMode || 'smart';\n      // destination may stay empty: backend then uses the portable downloads\\ folder.\n      const request = { ids, engine, destination, guardMode: mode };\n'''
s = replace_once(s, old, new, 'current download controls')
s = replace_once(s, "button.textContent = '🛡 Verific HDD + istoric + media…';", "button.textContent = '⏳ Verific și pregătesc…';", 'busy button label')
old = '''        await loadResults();\n        showGuardReport(data.guard, request, data.added);\n        showActivity(data.message || `${data.added || 0} fișier(e) confirmate ca lipsă au intrat în coadă.`, data.added > 0 ? 'ok' : 'info');\n        await loadQueue();\n'''
new = '''        await loadResults();\n        showGuardReport(data.guard, request, data.added);\n        const rejected = Array.isArray(data.rejected) ? data.rejected : [];\n        if (rejected.length) {\n          const list = document.getElementById('guardDecisionList');\n          if (list) {\n            const html = rejected.map(x => `<div class="guardItem" style="border-left:3px solid #ff6b7a"><div class="row"><b class="dangerText">NU S-A PORNIT</b><span class="guardAction">${esc(x.engine || 'auto')}</span></div><div style="margin-top:5px"><b>${esc(x.name || 'Fișier')}</b></div><div class="guardReason">${esc(x.title || 'Download indisponibil')}: ${esc(x.message || '')}</div>${x.action ? `<div class="muted small" style="margin-top:5px"><b>Ce faci:</b> ${esc(x.action)}</div>` : ''}</div>`).join('');\n            list.insertAdjacentHTML('afterbegin', html);\n          }\n        }\n        const firstReject = rejected[0];\n        const activity = data.message || `${data.added || 0} fișier(e) au intrat în coadă.`;\n        showActivity(firstReject && !data.added ? `${activity}. ${firstReject.title}: ${firstReject.action || firstReject.message}` : activity, data.added > 0 ? 'ok' : (rejected.length ? 'error' : 'info'));\n        await loadQueue();\n        if (data.added > 0) goTab('downloads');\n'''
s = replace_once(s, old, new, 'download response UX')
s = replace_once(s, "button.textContent = '🛡 Verifică inteligent + descarcă';", "button.textContent = '⬇ Descarcă selectate';", 'final button label')
p.write_text(s, encoding='utf-8', newline='\n')

# --- tests: snapshot JSON persistence and HTML rejection ---
p = Path('download_core_v855_test.go')
s = p.read_text(encoding='utf-8')
insert = r'''
func TestDownloadJobRemoteSnapshotJSONRoundTrip(t *testing.T) {
	job := DownloadJob{ID: "j1", ResultID: 7, Name: "v.mp4", Source: "MEGA", Remote: RemoteItem{Name: "v.mp4", Source: "MEGA", URL: "folder", Handle: "HANDLE7"}, RequestedEngine: "auto"}
	b, err := json.Marshal(job)
	if err != nil { t.Fatal(err) }
	var got DownloadJob
	if err := json.Unmarshal(b, &got); err != nil { t.Fatal(err) }
	if got.Remote.Handle != "HANDLE7" || got.RequestedEngine != "auto" {
		t.Fatalf("snapshot lost after JSON roundtrip: %#v", got)
	}
}

func TestInternalDownloadV855RejectsHTMLInsteadOfMedia(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, "<html>login</html>")
	}))
	defer srv.Close()
	res := Result{Remote: RemoteItem{Name: "clip.mp4", Source: "HTTP", URL: srv.URL, DirectURL: srv.URL, ContentType: "video/mp4"}}
	_, err := internalDownloadV855(context.Background(), res, t.TempDir(), func(int64, int64) {})
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "pagină html") {
		t.Fatalf("expected HTML response rejection, got %v", err)
	}
}
'''
marker = '\nvar modTimeV855 = time.Unix(1700000000, 0)\n'
if s.count(marker) != 1:
    raise SystemExit('test insertion marker missing/ambiguous')
s = s.replace(marker, insert + marker, 1)
# encoding/json is needed by roundtrip test.
s = replace_once(s, 'import (\n\t"context"\n', 'import (\n\t"context"\n\t"encoding/json"\n', 'json test import')
p.write_text(s, encoding='utf-8', newline='\n')

print('v8.5.5 download core integration applied')
