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
    'const appVersion = "8.4.0 Pro ExactGuard AI"',
    'const appVersion = "8.4.1 Pro ExactGuard AI Reliability"',
    "appVersion",
)
main = ensure_once(
    main,
    '\tmux.HandleFunc("/api/queue/action", a.handleQueueAction)\n',
    '\tmux.HandleFunc("/api/queue/action", a.handleQueueAction)\n'
    '\tmux.HandleFunc("/api/app/heartbeat", a.handleUIHeartbeat)\n'
    '\tmux.HandleFunc("/api/app/exit-hint", a.handleUIExitHint)\n',
    "lifecycle routes",
)
old_tail = '''\tgo func() {\n\t\ttime.Sleep(350 * time.Millisecond)\n\t\topenAppWindow(addr)\n\t}()\n\tif err := http.Serve(ln, mux); err != nil {\n\t\tlog.Fatal(err)\n\t}\n}\n'''
new_tail = '''\tshutdownCh := make(chan struct{}, 1)\n\tstartUIWatchdog(shutdownCh)\n\tsrv := &http.Server{Handler: mux}\n\tgo func() {\n\t\ttime.Sleep(350 * time.Millisecond)\n\t\topenAppWindow(addr)\n\t}()\n\tserveErr := make(chan error, 1)\n\tgo func() { serveErr <- srv.Serve(ln) }()\n\tselect {\n\tcase err := <-serveErr:\n\t\tif err != nil && !errors.Is(err, http.ErrServerClosed) {\n\t\t\tlog.Fatal(err)\n\t\t}\n\tcase <-shutdownCh:\n\t\ta.logf("Interfața aplicației s-a închis; opresc DDG controlat")\n\t\tshutdownApp(a)\n\t\tctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)\n\t\t_ = srv.Shutdown(ctx)\n\t\tcancel()\n\t}\n}\n'''
main = ensure_once(main, old_tail, new_tail, "server lifecycle")
main_path.write_text(main, encoding="utf-8")

web_path = Path("web/index.html")
web = web_path.read_text(encoding="utf-8")
web = ensure_once(
    web,
    "async function init(){let a=await api('/api/about');",
    "async function init(){await api('/api/app/heartbeat').catch(()=>{});let a=await api('/api/about');",
    "initial heartbeat",
)
web = ensure_once(
    web,
    "setInterval(()=>{if($('downloads')?.classList.contains('on'))loadQueue()},1200);}",
    "setInterval(()=>{if($('downloads')?.classList.contains('on'))loadQueue()},1200);setInterval(()=>api('/api/app/heartbeat').catch(()=>{}),1500);window.addEventListener('pagehide',()=>{try{navigator.sendBeacon('/api/app/exit-hint','')}catch{}});}",
    "heartbeat timer",
)
web_path.write_text(web, encoding="utf-8")

extra_path = Path("v8_extra.go")
extra = extra_path.read_text(encoding="utf-8")
old_cleanup = '''func removeAriaQueueJobsAsync(a *App, gids []string) {
	if len(gids) == 0 {
		return
	}
	go func(gids []string) {
		time.Sleep(150 * time.Millisecond)
		m, err := ariaRPCFor(a)
		if err != nil {
			return
		}
		for _, gid := range gids {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			_ = m.remove(ctx, gid)
			cancel()
		}
	}(append([]string(nil), gids...))
}
'''
new_cleanup = '''func removeAriaQueueJobsAsync(a *App, gids []string) {
	if len(gids) == 0 {
		return
	}
	// Cleanup must never start a fresh aria2 daemon merely to remove stale GIDs.
	// If the current RPC manager is already gone, the old daemon is gone too
	// (it is started with --stop-with-process), so there is nothing left to do.
	raw, ok := ariaRPCRegistry.Load(a)
	if !ok {
		return
	}
	m := raw.(*AriaRPCManager)
	go func(gids []string) {
		time.Sleep(150 * time.Millisecond)
		for _, gid := range gids {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			_ = m.remove(ctx, gid)
			cancel()
		}
	}(append([]string(nil), gids...))
}
'''
extra = ensure_once(extra, old_cleanup, new_cleanup, "aria cleanup without daemon spawn")
extra = ensure_once(
    extra,
    '\t\tif attempts > cfgRetries {\n',
    '\t\tif err == nil {\n\t\t\terr = errors.New("motorul de download nu a returnat nici fișier, nici eroare")\n\t\t}\n\t\tif attempts > cfgRetries {\n',
    "silent downloader result",
)
extra = ensure_once(
    extra,
    '\t\t\t\tx.Stage = "finalizat și verificat"\n',
    '\t\t\t\tx.Stage = "finalizat; validarea disponibilă a trecut"\n',
    "truthful completion stage",
)
extra = ensure_once(
    extra,
    '''\t\t\t\tj.Status = "paused"\n\t\t\t\tj.UpdatedAt = now\n\t\t\t\tj.GuardVersion = 0\n''',
    '''\t\t\t\tj.Status = "paused"\n\t\t\t\tj.Stage = "pus pe pauză"\n\t\t\t\tj.SpeedBps, j.ETA = 0, 0\n\t\t\t\tj.UpdatedAt = now\n\t\t\t\tj.GuardVersion = 0\n''',
    "pause state",
)
extra = ensure_once(
    extra,
    '''\t\t\t\tj.Status = "queued"\n\t\t\t\tj.Error = ""\n\t\t\t\tj.FinishedAt = 0\n\t\t\t\tj.UpdatedAt = now\n\t\t\t\tj.GuardVersion = 0\n''',
    '''\t\t\t\tj.Status = "queued"\n\t\t\t\tj.Error, j.ErrorCode, j.ErrorTitle, j.ErrorAction = "", "", "", ""\n\t\t\t\tj.Stage = "în așteptare"\n\t\t\t\tj.SpeedBps, j.ETA = 0, 0\n\t\t\t\tj.FinishedAt = 0\n\t\t\t\tj.UpdatedAt = now\n\t\t\t\tj.GuardVersion = 0\n''',
    "resume state",
)
extra = ensure_once(
    extra,
    '''\t\t\t\tj.Status = "cancelled"\n\t\t\t\tj.UpdatedAt = now\n\t\t\t\tj.FinishedAt = now\n''',
    '''\t\t\t\tj.Status = "cancelled"\n\t\t\t\tj.Stage = "oprit de utilizator"\n\t\t\t\tj.SpeedBps, j.ETA = 0, 0\n\t\t\t\tj.UpdatedAt = now\n\t\t\t\tj.FinishedAt = now\n''',
    "cancel state",
)
extra_path.write_text(extra, encoding="utf-8")

v7_path = Path("v7_extra.go")
v7 = v7_path.read_text(encoding="utf-8")
v7 = ensure_once(
    v7,
    '''func (a *App) runYtDlpDownload(ctx context.Context, exe, u, dest string) (string, error) {\n\tarchive := filepath.Join(a.appDir, "yt-dlp.archive.txt")\n\ta.mu.RLock()\n''',
    '''func (a *App) runYtDlpDownload(ctx context.Context, exe, u, dest string) (string, error) {\n\t// ExactGuard already protects explicit downloads. A historical yt-dlp\n\t// archive must not silently suppress a requested re-download after a file\n\t// was moved or removed from disk.\n\ta.mu.RLock()\n''',
    "yt-dlp stale archive declaration",
)
v7 = ensure_once(
    v7,
    '''\targs := []string{"--no-playlist", "--continue", "--no-overwrites", "--windows-filenames", "--download-archive", archive, "-P", dest, "--print", "after_move:filepath"}\n''',
    '''\targs := []string{"--no-playlist", "--continue", "--no-overwrites", "--windows-filenames", "-P", dest, "--print", "after_move:filepath"}\n''',
    "yt-dlp stale archive option",
)
old_verify = '''func verifyDownloadedAgainstRemote(path string, remote RemoteItem) error {
	if remote.Hash == "" || remote.HashType == "" {
		return nil
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	var got string
	switch strings.ToLower(remote.HashType) {
	case "sha256":
		h := sha256.New()
		if _, err = io.Copy(h, f); err == nil {
			got = hex.EncodeToString(h.Sum(nil))
		}
	case "md5":
		h := md5.New()
		if _, err = io.Copy(h, f); err == nil {
			got = hex.EncodeToString(h.Sum(nil))
		}
	default:
		return nil
	}
	if err != nil {
		return err
	}
	if !strings.EqualFold(got, remote.Hash) {
		return fmt.Errorf("checksum %s diferit după download", remote.HashType)
	}
	return nil
}
'''
new_verify = '''func verifyDownloadedAgainstRemote(path string, remote RemoteItem) error {
	st, err := os.Stat(path)
	if err != nil {
		return err
	}
	if st.IsDir() {
		return errors.New("rezultatul downloadului este un folder, nu un fișier")
	}
	if remote.Size > 0 && !remote.ApproxSize && st.Size() != remote.Size {
		return fmt.Errorf("dimensiune diferită după download: local %d bytes, remote %d bytes", st.Size(), remote.Size)
	}
	if remote.Hash == "" || remote.HashType == "" {
		return nil
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	var got string
	switch strings.ToLower(remote.HashType) {
	case "sha256":
		h := sha256.New()
		if _, err = io.Copy(h, f); err == nil {
			got = hex.EncodeToString(h.Sum(nil))
		}
	case "md5":
		h := md5.New()
		if _, err = io.Copy(h, f); err == nil {
			got = hex.EncodeToString(h.Sum(nil))
		}
	default:
		return nil
	}
	if err != nil {
		return err
	}
	if !strings.EqualFold(got, remote.Hash) {
		return fmt.Errorf("checksum %s diferit după download", remote.HashType)
	}
	return nil
}
'''
v7 = ensure_once(v7, old_verify, new_verify, "download size verification")
old_direct_finish = '''\t\tif e != nil {
			a.logf("Download eșuat %s: %v", x.Remote.Name, e)
			a.failOp("Download eșuat: "+x.Remote.Name, e.Error())
			return
		}
		if path != "" {
			if _, statErr := os.Stat(path); statErr == nil {
				if verifyErr := verifyDownloadedAgainstRemote(path, x.Remote); verifyErr != nil {
					bad := path + ".checksum_failed"
					_ = os.Rename(path, bad)
					a.failOp("Download invalid: "+x.Remote.Name, verifyErr.Error())
					return
				}
				a.markDownloaded(x.ID, path)
			}
		}
'''
new_direct_finish = '''\t\tif e == nil && path == "" {
			e = errors.New("motorul de download nu a returnat nici fișier, nici eroare")
		}
		if e != nil {
			a.logf("Download eșuat %s: %v", x.Remote.Name, e)
			a.failOp("Download eșuat: "+x.Remote.Name, e.Error())
			return
		}
		if _, statErr := os.Stat(path); statErr != nil {
			a.failOp("Download invalid: "+x.Remote.Name, "Motorul a raportat succes, dar fișierul rezultat nu există: "+statErr.Error())
			return
		}
		if verifyErr := verifyDownloadedAgainstRemote(path, x.Remote); verifyErr != nil {
			bad := path + ".verification_failed"
			_ = os.Rename(path, bad)
			a.failOp("Download invalid: "+x.Remote.Name, verifyErr.Error())
			return
		}
		a.markDownloaded(x.ID, path)
'''
v7 = ensure_once(v7, old_direct_finish, new_direct_finish, "direct download completion verification")
v7_path.write_text(v7, encoding="utf-8")
