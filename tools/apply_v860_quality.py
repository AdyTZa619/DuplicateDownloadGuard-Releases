from pathlib import Path

q = Path('quality_intelligence_v860.go')
s = q.read_text(encoding='utf-8')
s = s.replace('\n\t"errors"\n', '\n')
q.write_text(s, encoding='utf-8', newline='\n')

main = Path('main.go')
m = main.read_text(encoding='utf-8')
route = '\tmux.HandleFunc("/api/media/quality", a.handleMediaQualityV860)\n'
needle = '\tmux.HandleFunc("/api/media/compare", a.handleMediaCompare)\n'
if route not in m:
    if needle not in m:
        raise SystemExit('media compare route marker not found')
    m = m.replace(needle, needle + route, 1)
    main.write_text(m, encoding='utf-8', newline='\n')

index = Path('web/index.html')
h = index.read_text(encoding='utf-8')
script = '<script defer src="/quality_intelligence_v860.js"></script>'
marker = '<script defer src="/download_diagnostic_v860.js"></script>'
if script not in h:
    if marker not in h:
        raise SystemExit('download diagnostic script marker not found')
    h = h.replace(marker, marker + script, 1)
    index.write_text(h, encoding='utf-8', newline='\n')
print('quality intelligence integrated')
