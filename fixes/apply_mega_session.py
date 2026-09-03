from pathlib import Path


def ensure_once(text: str, old: str, new: str, label: str) -> str:
    old_count = text.count(old)
    new_count = text.count(new)
    if old_count == 1 and new_count == 0:
        return text.replace(old, new, 1)
    if old_count == 0 and new_count == 1:
        return text
    raise SystemExit(f"{label}: unexpected state old={old_count} new={new_count}")


main_path = Path("main.go")
main = main_path.read_text(encoding="utf-8")

main = ensure_once(
    main,
    '\ta.logf("MEGA: scanare folder public")\n',
    '''\ta.updateProgress(func(p *Progress) {\n\t\tp.State = "running"\n\t\tp.Message = "MEGA • aștept sesiunea exclusivă"\n\t\tp.Detail = "Scanarea, preview-ul și downloadul MEGA folosesc pe rând aceeași sesiune pentru a evita logout/login concurent."\n\t})\n\tif err := acquireMegaSession(ctx); err != nil {\n\t\ta.failOp("MEGA: operație anulată", "Nu am putut obține sesiunea MEGA: "+err.Error())\n\t\treturn\n\t}\n\tdefer releaseMegaSession()\n\tif err := a.stopMegaPreviewWhileSessionOwned("pornire scanare MEGA"); err != nil {\n\t\ta.logf("MEGA: cleanup preview înainte de scanare: %v", err)\n\t}\n\ta.logf("MEGA: scanare folder public")\n''',
    "serialize MEGA scan",
)

main = ensure_once(
    main,
    '''func (a *App) stopMegaPreview(reason string) error {\n\ta.previewMu.Lock()\n\tdefer a.previewMu.Unlock()\n\treturn a.stopMegaPreviewLocked(reason)\n}\n''',
    '''func (a *App) stopMegaPreview(reason string) error {\n\t// Fast path: most Stop calls arrive after another MEGA operation has already\n\t// cleaned the preview. Do not wait for a long download when there is nothing\n\t// left to stop.\n\ta.previewMu.Lock()\n\tactive := a.preview.Active\n\ta.previewMu.Unlock()\n\tif !active {\n\t\treturn nil\n\t}\n\tgateCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)\n\tdefer cancel()\n\tif err := acquireMegaSession(gateCtx); err != nil {\n\t\treturn fmt.Errorf("MEGA este ocupat cu altă operație; preview-ul nu a putut fi oprit încă: %w", err)\n\t}\n\tdefer releaseMegaSession()\n\ta.previewMu.Lock()\n\tdefer a.previewMu.Unlock()\n\treturn a.stopMegaPreviewLocked(reason)\n}\n''',
    "serialize preview stop",
)

main = ensure_once(
    main,
    '''func (a *App) startMegaPreview(item RemoteItem) (string, error) {\n\ta.previewMu.Lock()\n\tdefer a.previewMu.Unlock()\n''',
    '''func (a *App) startMegaPreview(item RemoteItem) (string, error) {\n\tgateCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)\n\tdefer cancel()\n\tif err := acquireMegaSession(gateCtx); err != nil {\n\t\treturn "", fmt.Errorf("MEGA este ocupat cu scanare sau download; încearcă preview-ul din nou după terminarea operației: %w", err)\n\t}\n\tdefer releaseMegaSession()\n\ta.previewMu.Lock()\n\tdefer a.previewMu.Unlock()\n''',
    "serialize preview start",
)

main_path.write_text(main, encoding="utf-8")

v7_path = Path("v7_extra.go")
v7 = v7_path.read_text(encoding="utf-8")
v7 = ensure_once(
    v7,
    '''func (a *App) downloadMegaResults(ctx context.Context, rows []Result, dest string) error {\n\tif len(rows) == 0 {\n\t\treturn nil\n\t}\n\texe := a.detectMegaClient()\n''',
    '''func (a *App) downloadMegaResults(ctx context.Context, rows []Result, dest string) error {\n\tif len(rows) == 0 {\n\t\treturn nil\n\t}\n\tif err := acquireMegaSession(ctx); err != nil {\n\t\treturn err\n\t}\n\tdefer releaseMegaSession()\n\t// A running WebDAV preview owns the public-folder session. Stop/restore it\n\t// before MEGAcmd login/get so a preview can never invalidate a download.\n\tif err := a.stopMegaPreviewWhileSessionOwned("pornire download MEGA"); err != nil {\n\t\ta.logf("MEGA: cleanup preview înainte de download: %v", err)\n\t}\n\texe := a.detectMegaClient()\n''',
    "serialize MEGA download",
)
v7_path.write_text(v7, encoding="utf-8")

v8_path = Path("v8_extra.go")
v8 = v8_path.read_text(encoding="utf-8")
v8 = ensure_once(
    v8,
    '''\t\tcase "mega":\n\t\t\tq.update(a, id, func(x *DownloadJob) { x.Stage = "MEGAcmd descarcă fișierul" })\n\t\t\tmegaQueueMu.Lock()\n\t\t\tif a.opRunning.Load() {\n\t\t\t\tmegaQueueMu.Unlock()\n\t\t\t\terr = errors.New("MEGA este ocupat cu scanare/preview; retry automat")\n\t\t\t} else {\n\t\t\t\terr = a.downloadMegaResults(ctx, []Result{res}, dest)\n\t\t\t\tif err == nil {\n\t\t\t\t\tpath = findDownloadedMegaFile(dest, res, start)\n\t\t\t\t\tif path == "" {\n\t\t\t\t\t\terr = errors.New("MEGAcmd a terminat fără eroare, dar fișierul rezultat nu a fost găsit în folderul de download")\n\t\t\t\t\t}\n\t\t\t\t}\n\t\t\t\tmegaQueueMu.Unlock()\n\t\t\t}\n''',
    '''\t\tcase "mega":\n\t\t\tq.update(a, id, func(x *DownloadJob) { x.Stage = "MEGAcmd așteaptă sesiunea / descarcă fișierul" })\n\t\t\t// Do not burn retry attempts merely because a scan is active. The\n\t\t\t// cancellable MEGA session gate waits safely until scan/preview releases\n\t\t\t// the single MEGAcmd session.\n\t\t\tmegaQueueMu.Lock()\n\t\t\terr = a.downloadMegaResults(ctx, []Result{res}, dest)\n\t\t\tif err == nil {\n\t\t\t\tpath = findDownloadedMegaFile(dest, res, start)\n\t\t\t\tif path == "" {\n\t\t\t\t\terr = errors.New("MEGAcmd a terminat fără eroare, dar fișierul rezultat nu a fost găsit în folderul de download")\n\t\t\t\t}\n\t\t\t}\n\t\t\tmegaQueueMu.Unlock()\n''',
    "queue waits for MEGA session",
)
v8_path.write_text(v8, encoding="utf-8")
