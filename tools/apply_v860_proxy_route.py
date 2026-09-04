from pathlib import Path

p = Path('main.go')
s = p.read_text(encoding='utf-8')
needle = '\tmux.HandleFunc("/api/remote-preview/start", a.handleRemotePreviewStart)\n'
route = '\tmux.HandleFunc("/api/remote-preview/proxy", a.handleRemotePreviewProxyV860)\n'
if route in s:
    print('route already present')
elif needle not in s:
    raise SystemExit('expected remote-preview/start route not found')
else:
    s = s.replace(needle, needle + route, 1)
    p.write_text(s, encoding='utf-8', newline='\n')
    print('proxy route inserted')
