from pathlib import Path

main = Path('main.go')
s = main.read_text(encoding='utf-8')
route = '\tmux.HandleFunc("/api/download/diagnostic", a.handleDownloadDiagnosticV860)\n'
needle = '\tmux.HandleFunc("/api/download/preflight", a.handleDownloadPreflight)\n'
if route not in s:
    if needle not in s:
        raise SystemExit('download preflight route marker not found')
    s = s.replace(needle, needle + route, 1)
    main.write_text(s, encoding='utf-8', newline='\n')
    print('diagnostic API route inserted')
else:
    print('diagnostic API route already present')

index = Path('web/index.html')
h = index.read_text(encoding='utf-8')
script = '<script defer src="/download_diagnostic_v860.js"></script>'
marker = '<script defer src="/preview_quick_v86.js"></script>'
if script not in h:
    if marker not in h:
        raise SystemExit('preview_quick script marker not found')
    h = h.replace(marker, marker + script, 1)
    index.write_text(h, encoding='utf-8', newline='\n')
    print('diagnostic UI script inserted')
else:
    print('diagnostic UI script already present')
