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
extra_path.write_text(extra, encoding="utf-8")
