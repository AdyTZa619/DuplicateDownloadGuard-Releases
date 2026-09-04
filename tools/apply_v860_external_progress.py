from pathlib import Path

p = Path('v8_extra.go')
s = p.read_text(encoding='utf-8')
old_yt = 'path, err = a.runYtDlpDownload(ctx, exe, ytDlpInputURLV855(res), dest)'
new_yt = 'path, err = a.runYtDlpDownloadProgressV860(ctx, exe, ytDlpInputURLV855(res), dest, progress)'
if new_yt not in s:
    if old_yt not in s:
        raise SystemExit('yt-dlp queue call marker not found')
    s = s.replace(old_yt, new_yt, 1)
    print('yt-dlp structured progress integrated')

old_mega = '''\t\t\tmegaQueueMu.Lock()\n\t\t\terr = a.downloadMegaResults(ctx, []Result{megaRes}, dest)'''
new_mega = '''\t\t\tmegaQueueMu.Lock()\n\t\t\tmegaWatchCtxV860, megaWatchCancelV860 := context.WithCancel(ctx)\n\t\t\tmegaWatchDoneV860 := make(chan struct{})\n\t\t\tgo func() {\n\t\t\t\tdefer close(megaWatchDoneV860)\n\t\t\t\twatchExternalDownloadDirectoryV860(megaWatchCtxV860, dest, res.Remote.Name, res.Remote.Size, progress)\n\t\t\t}()\n\t\t\terr = a.downloadMegaResults(ctx, []Result{megaRes}, dest)\n\t\t\tmegaWatchCancelV860()\n\t\t\t<-megaWatchDoneV860'''
if 'megaWatchCtxV860' not in s:
    if old_mega not in s:
        raise SystemExit('MEGA queue download marker not found')
    s = s.replace(old_mega, new_mega, 1)
    print('MEGA directory progress observer integrated')
p.write_text(s, encoding='utf-8', newline='\n')
