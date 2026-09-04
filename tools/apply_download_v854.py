from pathlib import Path


def replace_once(path, old, new):
    p = Path(path)
    s = p.read_text(encoding='utf-8')
    if old not in s:
        raise SystemExit(f'pattern missing in {path}: {old[:120]!r}')
    if s.count(old) != 1:
        raise SystemExit(f'pattern count {s.count(old)} in {path}: {old[:120]!r}')
    p.write_text(s.replace(old, new, 1), encoding='utf-8')

# Persist the complete remote source with each queue job.
replace_once('v8_extra.go',
'''\tName          string `json:"name"`\n\tSource        string `json:"source"`\n\tURL           string `json:"url,omitempty"`\n\tDestination   string `json:"destination"`\n\tEngine        string `json:"engine"`\n''',
'''\tName          string     `json:"name"`\n\tSource        string     `json:"source"`\n\tURL           string     `json:"url,omitempty"`\n\tRemote        RemoteItem `json:"remote,omitempty"`\n\tDestination   string     `json:"destination"`\n\tEngine        string     `json:"engine"`\n\tEngineReason  string     `json:"engineReason,omitempty"`\n''')

replace_once('v8_extra.go',
'''func chooseQueueEngine(a *App, res Result, requested string) string {\n\te := strings.ToLower(strings.TrimSpace(requested))\n\tif e != "" && e != "auto" {\n\t\treturn e\n\t}\n\tif strings.EqualFold(res.Remote.Source, "MEGA") {\n\t\treturn "mega"\n\t}\n\tif strings.EqualFold(res.Remote.Source, "YT-DLP") && a.detectYtDlp() != "" {\n\t\treturn "yt-dlp"\n\t}\n\tif a.detectAria2() != "" && resultDownloadURL(res) != "" {\n\t\treturn "aria2"\n\t}\n\treturn "internal"\n}\n''',
'''func chooseQueueEngine(a *App, res Result, requested string) string {\n\tplan, err := chooseDownloadPlanV854(a, res, requested)\n\tif err != nil {\n\t\treturn "unavailable"\n\t}\n\treturn plan.Engine\n}\n''')

replace_once('v8_extra.go',
'''\tq.mu.Lock()\n\tj := q.findLocked(id)\n\tif j == nil {\n\t\tq.mu.Unlock()\n\t\treturn\n\t}\n\trid, engine, dest := j.ResultID, j.Engine, j.Destination\n\tq.mu.Unlock()\n\tres, ok := a.resultByID(rid)\n\tif !ok {\n\t\tq.update(a, id, func(x *DownloadJob) { x.Status = "failed"; x.Error = "rezultatul sursă nu mai există" })\n\t\treturn\n\t}\n''',
'''\tq.mu.Lock()\n\tj := q.findLocked(id)\n\tif j == nil {\n\t\tq.mu.Unlock()\n\t\treturn\n\t}\n\tjobSnapshot := *j\n\trid, engine, dest := j.ResultID, j.Engine, j.Destination\n\tq.mu.Unlock()\n\tvar live *Result\n\tif current, ok := a.resultByID(rid); ok {\n\t\tlive = &current\n\t}\n\tres, sourceErr := resultFromDownloadJobV854(jobSnapshot, live)\n\tif sourceErr != nil {\n\t\tq.update(a, id, func(x *DownloadJob) {\n\t\t\tx.Status = "failed"\n\t\t\tx.ErrorCode = "SOURCE_SNAPSHOT_MISSING"\n\t\t\tx.ErrorTitle = "Sursa jobului nu mai poate fi reconstruită"\n\t\t\tx.Error = sourceErr.Error()\n\t\t\tx.ErrorAction = "Selectează din nou fișierul în Rezultate și apasă Descarcă selecția."\n\t\t\tx.Stage = "oprit înainte de pornirea motorului"\n\t\t\tx.FinishedAt = time.Now().Unix()\n\t\t})\n\t\treturn\n\t}\n\tif strings.TrimSpace(jobSnapshot.Remote.Name) == "" && live != nil {\n\t\tq.update(a, id, func(x *DownloadJob) { x.Remote = res.Remote })\n\t}\n''')

replace_once('v8_extra.go',
'''\tengine = chooseQueueEngine(a, res, engine)\n\tq.update(a, id, func(x *DownloadJob) {\n\t\tx.Engine = engine\n\t\tx.BytesTotal = res.Remote.Size\n\t\tif x.MaxRetries <= 0 {\n\t\t\tx.MaxRetries = 3\n\t\t}\n\t})\n\ta.mu.RLock()\n\tcfgRetries := a.cfg.DownloadRetries\n\ta.mu.RUnlock()\n\tif cfgRetries <= 0 {\n\t\tcfgRetries = 3\n\t}\n''',
'''\tplan, planErr := chooseDownloadPlanV854(a, res, engine)\n\tif planErr != nil {\n\t\tq.update(a, id, func(x *DownloadJob) {\n\t\t\tx.Status = "failed"\n\t\t\tx.ErrorCode = "ENGINE_NOT_READY"\n\t\t\tx.ErrorTitle = "Nu există un motor potrivit pentru această sursă"\n\t\t\tx.Error = planErr.Error()\n\t\t\tx.ErrorAction = "Folosește Auto sau instalează motorul indicat în AI & Tool Manager."\n\t\t\tx.Stage = "oprit la alegerea motorului"\n\t\t\tx.FinishedAt = time.Now().Unix()\n\t\t})\n\t\treturn\n\t}\n\tengine = plan.Engine\n\tq.update(a, id, func(x *DownloadJob) {\n\t\tx.Engine = engine\n\t\tx.EngineReason = plan.Reason\n\t\tx.BytesTotal = res.Remote.Size\n\t\tx.Stage = "pregătit • " + plan.Reason\n\t})\n\ta.mu.RLock()\n\tcfgRetries := a.cfg.DownloadRetries\n\ta.mu.RUnlock()\n\tif cfgRetries <= 0 {\n\t\tcfgRetries = 3\n\t}\n\tq.update(a, id, func(x *DownloadJob) { x.MaxRetries = cfgRetries + 1 })\n''')

replace_once('v8_extra.go',
'''\t\tdefault:\n\t\t\tu := resultDownloadURL(res)\n\t\t\tif u == "" {\n\t\t\t\terr = errors.New("URL direct lipsă")\n\t\t\t} else {\n\t\t\t\tpath, err = internalDownload(ctx, u, dest, res.Remote.Name, progress)\n\t\t\t}\n\t\t}\n''',
'''\t\tdefault:\n\t\t\tpath, err = internalDownloadV854(ctx, res.Remote, dest, res.Remote.Name, progress)\n\t\t}\n''')

# Normalize retries before queue creation and plan engines before taking queue lock.
replace_once('v8_extra.go',
'''\tif err := os.MkdirAll(dest, 0755); err != nil {\n\t\thttp.Error(w, err.Error(), 500)\n\t\treturn\n\t}\n\tselectedRows := selectedResults(rows, req.IDs)\n''',
'''\tif err := os.MkdirAll(dest, 0755); err != nil {\n\t\thttp.Error(w, err.Error(), 500)\n\t\treturn\n\t}\n\tif retries <= 0 {\n\t\tretries = 3\n\t}\n\tselectedRows := selectedResults(rows, req.IDs)\n''')

replace_once('v8_extra.go',
'''\tq := queueFor(a)\n\tadded := 0\n\tariaRemove := []string{}\n\tq.mu.Lock()\n''',
'''\tplans := map[int]downloadPlanV854{}\n\trejected := []downloadRejectionV854{}\n\tfor _, res := range selectedRows {\n\t\tif !wanted[res.ID] {\n\t\t\tcontinue\n\t\t}\n\t\tplan, planErr := chooseDownloadPlanV854(a, res, req.Engine)\n\t\tif planErr != nil {\n\t\t\trejected = append(rejected, downloadRejectionV854{ResultID: res.ID, Name: res.Remote.Name, Reason: planErr.Error()})\n\t\t\tdelete(wanted, res.ID)\n\t\t\tcontinue\n\t\t}\n\t\tplans[res.ID] = plan\n\t}\n\tq := queueFor(a)\n\tadded := 0\n\tariaRemove := []string{}\n\tq.mu.Lock()\n''')

replace_once('v8_extra.go',
'''\t\tdecision := decisions[res.ID]\n\t\tnow := time.Now().Unix()\n\t\tq.Jobs = append(q.Jobs, &DownloadJob{ID: jid, ResultID: res.ID, Name: res.Remote.Name, Source: res.Remote.Source, URL: resultDownloadURL(res), Destination: dest, Engine: chooseQueueEngine(a, res, req.Engine), Status: "queued", Priority: 0, BytesTotal: res.Remote.Size, MaxRetries: retries, GuardMode: report.Mode, GuardVerdict: decision.Verdict, GuardReason: decision.Reason, GuardMethod: decision.Method, GuardVersion: downloadGuardVersion, GuardAt: now, GuardOverride: decision.Verdict == guardReview && req.AllowReview, AddedAt: now, UpdatedAt: now})\n''',
'''\t\tdecision := decisions[res.ID]\n\t\tplan := plans[res.ID]\n\t\tnow := time.Now().Unix()\n\t\tq.Jobs = append(q.Jobs, &DownloadJob{ID: jid, ResultID: res.ID, Name: res.Remote.Name, Source: res.Remote.Source, URL: resultDownloadURL(res), Remote: res.Remote, Destination: dest, Engine: plan.Engine, EngineReason: plan.Reason, Status: "queued", Stage: "pregătit • " + plan.Reason, Priority: 0, BytesTotal: res.Remote.Size, MaxRetries: retries + 1, GuardMode: report.Mode, GuardVerdict: decision.Verdict, GuardReason: decision.Reason, GuardMethod: decision.Method, GuardVersion: downloadGuardVersion, GuardAt: now, GuardOverride: decision.Verdict == guardReview && req.AllowReview, AddedAt: now, UpdatedAt: now})\n''')

replace_once('v8_extra.go',
'''\tmessage := fmt.Sprintf("%d adăugate în coadă • %d duplicate blocate • %d necesită review", added, duplicates, review)\n\tjsonOut(w, map[string]any{"ok": true, "added": added, "destination": dest, "guard": report, "reviewOverride": req.AllowReview, "message": message})\n''',
'''\tmessage := fmt.Sprintf("%d adăugate în coadă • %d duplicate blocate • %d necesită review • %d respinse de motor", added, duplicates, review, len(rejected))\n\tjsonOut(w, map[string]any{"ok": true, "added": added, "rejected": rejected, "destination": dest, "guard": report, "reviewOverride": req.AllowReview, "message": message})\n''')

# Explicit aria2 gets the same browser/referer context as the internal engine.
replace_once('aria2_rpc.go',
'''\topt := map[string]string{"dir": dest, "out": sanitizeFilename(res.Remote.Name), "continue": "true", "auto-file-renaming": "false", "allow-overwrite": "false", "file-allocation": "none", "split": strconv.Itoa(conn), "max-connection-per-server": strconv.Itoa(conn), "min-split-size": "1M", "max-tries": strconv.Itoa(retries + 1), "retry-wait": "2"}\n''',
'''\topt := map[string]string{"dir": dest, "out": sanitizeFilename(res.Remote.Name), "continue": "true", "auto-file-renaming": "false", "allow-overwrite": "false", "file-allocation": "none", "split": strconv.Itoa(conn), "max-connection-per-server": strconv.Itoa(conn), "min-split-size": "1M", "max-tries": strconv.Itoa(retries + 1), "retry-wait": "2", "user-agent": browserUserAgentV854()}\n\tif ref := downloadRefererV854(res.Remote); ref != "" {\n\t\topt["referer"] = ref\n\t}\n''')

# Make the download UX use live form values, portable default destination and immediate queue feedback.
replace_once('web/exact_guard.js',
'''      downloadButton.textContent = '🛡 Verifică inteligent + descarcă';\n      downloadButton.title = 'Rescanează HDD-urile, verifică istoricul, hash-ul și variantele media înainte de orice download';\n''',
'''      downloadButton.textContent = '⬇ Descarcă selecția';\n      downloadButton.title = 'Descarcă selecția. Smart Guard rulează automat înainte de coadă; Auto alege un motor sigur după tipul sursei.';\n''')

old_download = '''    downloadSelected = async function () {\n      const ids = idsForAction();\n      if (!ids.length) return toast('Selectează fișiere');\n      const destination = cfg.downloadDir || document.getElementById('downloadDir')?.value || '';\n      if (!destination) return toast('Setează folderul de download');\n      const mode = document.getElementById('downloadGuardMode')?.value || cfg.downloadGuardMode || 'smart';\n      const request = { ids, engine: cfg.downloadMethod || 'auto', destination, guardMode: mode };\n      const button = document.getElementById('downloadGuardBtn');\n      if (button) {\n        button.disabled = true;\n        button.textContent = '🛡 Verific HDD + istoric + media…';\n      }\n      const started = Date.now();\n      showActivity(`Verificarea a început pentru ${ids.length} fișier(e): index live, istoric, hash și candidați media…`);\n      clearInterval(guardTicker);\n      guardTicker = setInterval(() => {\n        const seconds = Math.floor((Date.now() - started) / 1000);\n        showActivity(`Verificare în curs: ${seconds}s • ${ids.length} fișier(e). Cazurile media dificile pot necesita ffprobe/fingerprint.`);\n      }, 1000);\n      toast('Smart Guard verifică dacă fișierele chiar lipsesc…');\n      try {\n        const data = await api('/api/queue/add', {\n          method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(request)\n        });\n        await loadResults();\n        showGuardReport(data.guard, request, data.added);\n        showActivity(data.message || `${data.added || 0} fișier(e) confirmate ca lipsă au intrat în coadă.`, data.added > 0 ? 'ok' : 'info');\n        await loadQueue();\n      } catch (error) {\n        showActivity(`Download oprit cu eroare: ${error.message}`, 'error');\n        toast(error.message);\n      } finally {\n        clearInterval(guardTicker);\n        guardTicker = null;\n        if (button) {\n          button.disabled = false;\n          button.textContent = '🛡 Verifică inteligent + descarcă';\n        }\n      }\n    };\n'''
new_download = '''    downloadSelected = async function () {\n      const ids = idsForAction();\n      if (!ids.length) return toast('Selectează cel puțin un fișier');\n      const destination = document.getElementById('downloadDir')?.value?.trim() || cfg.downloadDir || '';\n      const engine = document.getElementById('downloadMethod')?.value || cfg.downloadMethod || 'auto';\n      const mode = document.getElementById('downloadGuardMode')?.value || cfg.downloadGuardMode || 'smart';\n      const request = { ids, engine, destination, guardMode: mode };\n      const button = document.getElementById('downloadGuardBtn');\n      if (button) {\n        button.disabled = true;\n        button.textContent = '⬇ Pregătesc descărcarea…';\n      }\n      showActivity(`Pregătesc ${ids.length} fișier(e) • motor ${engine === 'auto' ? 'Auto' : engine} • verific duplicatele înainte de coadă…`);\n      toast('Pregătesc descărcarea…');\n      try {\n        const data = await api('/api/queue/add', {\n          method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(request)\n        });\n        await loadResults();\n        const rejected = Array.isArray(data.rejected) ? data.rejected : [];\n        const counts = data.guard?.counts || {};\n        if ((data.added || 0) > 0) {\n          await loadQueue();\n          goTab('downloads');\n          const extra = rejected.length ? ` • ${rejected.length} respins(e): ${rejected[0].reason}` : '';\n          showActivity(`${data.added} fișier(e) au intrat în coadă${extra}`, rejected.length ? 'info' : 'ok');\n          toast(`${data.added} fișier(e) în coadă${rejected.length ? ` • ${rejected.length} respinse` : ''}`);\n          if ((counts.REVIEW || 0) > 0) showGuardReport(data.guard, request, data.added);\n        } else {\n          showGuardReport(data.guard, request, 0);\n          if (rejected.length) {\n            showActivity(`Nu am pornit downloadul: ${rejected[0].reason}`, 'error');\n            toast(rejected[0].reason);\n          } else {\n            showActivity(data.message || 'Niciun fișier nu a intrat în coadă. Verifică verdictul Smart Guard.', 'info');\n          }\n        }\n      } catch (error) {\n        showActivity(`Download oprit: ${error.message}`, 'error');\n        toast(error.message);\n      } finally {\n        if (button) {\n          button.disabled = false;\n          button.textContent = '⬇ Descarcă selecția';\n        }\n      }\n    };\n'''
replace_once('web/exact_guard.js', old_download, new_download)

replace_once('web/exact_guard.js',
'''    qStatusLabel = function (status) {\n      return ({ queued: 'ÎN COADĂ', running: 'DESCARCĂ', paused: 'PAUZĂ', completed: 'GATA', failed: 'EROARE', cancelled: 'ANULAT', blocked: 'NU DESCĂRCA' })[status] || status;\n    };\n''',
'''    qStatusLabel = function (status) {\n      return ({ queued: 'ÎN AȘTEPTARE', running: 'SE DESCARCĂ', paused: 'PAUZĂ', completed: 'GATA', failed: 'EROARE', cancelled: 'ANULAT', blocked: 'NU DESCĂRCA' })[status] || status;\n    };\n''')

replace_once('web/exact_guard.js',
'''</td><td>${esc(job.engine || 'auto')}${job.gid ? `<div class="muted small">GID ${esc(job.gid)}</div>` : ''}</td><td><div class="downloadBar">''',
'''</td><td>${esc(job.engine || 'auto')}${job.engineReason ? `<div class="muted small">${esc(job.engineReason)}</div>` : ''}${job.gid ? `<div class="muted small">GID ${esc(job.gid)}</div>` : ''}</td><td><div class="downloadBar">''')

# Keep help aligned with the simpler UX.
replace_once('web/exact_guard.js',
'''        '<div class="helpStep"><div>„🛡 Verifică inteligent + descarcă” rescanează live locațiile și verifică mai întâi dacă fișierul a fost deja descărcat.</div></div>' +\n''',
'''        '<div class="helpStep"><div>„⬇ Descarcă selecția” verifică automat duplicatele și pune imediat în coadă doar fișierele care pot fi descărcate în siguranță.</div></div>' +\n''')

# The temporary workflow/script remove themselves after a successful patch.
Path('tools/apply_download_v854.py').unlink(missing_ok=True)
Path('.github/workflows/apply-download-v854.yml').unlink(missing_ok=True)
print('download v8.5.4 patch applied')
