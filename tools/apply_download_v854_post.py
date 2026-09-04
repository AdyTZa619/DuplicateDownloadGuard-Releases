from pathlib import Path


def replace_once(path, old, new):
    p = Path(path)
    s = p.read_text(encoding='utf-8')
    if old not in s:
        raise SystemExit(f'pattern missing in {path}: {old[:140]!r}')
    if s.count(old) != 1:
        raise SystemExit(f'pattern count {s.count(old)} in {path}: {old[:140]!r}')
    p.write_text(s.replace(old, new, 1), encoding='utf-8')

# yt-dlp must receive the stable source page whenever available, not a stale
# CDN URL saved during discovery.
replace_once('v8_extra.go',
'''\t\tcase "yt-dlp":\n\t\t\texe := a.detectYtDlp()\n\t\t\tif exe == "" {\n\t\t\t\terr = errors.New("yt-dlp lipsește")\n\t\t\t} else {\n\t\t\t\tpath, err = a.runYtDlpDownload(ctx, exe, res.Remote.URL, dest)\n\t\t\t}\n''',
'''\t\tcase "yt-dlp":\n\t\t\texe := a.detectYtDlp()\n\t\t\tinput := ytDlpInputV854(res.Remote)\n\t\t\tif exe == "" {\n\t\t\t\terr = errors.New("yt-dlp lipsește")\n\t\t\t} else if input == "" {\n\t\t\t\terr = errors.New("yt-dlp nu are URL sursă")\n\t\t\t} else {\n\t\t\t\tpath, err = a.runYtDlpDownload(ctx, exe, input, dest)\n\t\t\t}\n''')

# The queue UI must show the real portable fallback folder when no custom
# download folder is configured.
replace_once('v8_extra.go',
'''\tjsonOut(w, map[string]any{"jobs": rows, "summary": queueSummary(rows), "megaStatus": megaStatus, "downloadDir": func() string { a.mu.RLock(); defer a.mu.RUnlock(); return a.cfg.DownloadDir }()})\n''',
'''\tdownloadDir := func() string {\n\t\ta.mu.RLock()\n\t\td := strings.TrimSpace(a.cfg.DownloadDir)\n\t\ta.mu.RUnlock()\n\t\tif d == "" {\n\t\t\td = portableDownloadsDir()\n\t\t}\n\t\treturn d\n\t}()\n\tjsonOut(w, map[string]any{"jobs": rows, "summary": queueSummary(rows), "megaStatus": megaStatus, "downloadDir": downloadDir})\n''')

# Remove the last stale label from the empty queue state.
replace_once('web/exact_guard.js',
'''Coada este goală. Selectează rezultate și apasă „🛡 Verifică inteligent + descarcă”.''',
'''Coada este goală. Selectează rezultate și apasă „⬇ Descarcă selecția”.''')

Path('tools/apply_download_v854_post.py').unlink(missing_ok=True)
print('download v8.5.4 post patch applied')
