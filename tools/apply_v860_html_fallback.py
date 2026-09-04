from pathlib import Path

p = Path('v7_extra.go')
s = p.read_text(encoding='utf-8')
marker = 'if len(items) == 0 && (adapter == "auto" || adapter == "gallery-dl") {'
insert = '''\n\tif len(items) == 0 && (adapter == "auto" || adapter == "html") {\n\t\tif x, e := a.probeHTMLMediaV860(ctx, req.URL); e == nil {\n\t\t\titems = x\n\t\t\tused = "html"\n\t\t} else {\n\t\t\terrs = append(errs, "HTML: "+e.Error())\n\t\t}\n\t}\n'''
if 'used = "html"' in s and 'probeHTMLMediaV860' in s:
    print('HTML fallback already integrated')
else:
    start = s.find(marker)
    if start < 0:
        raise SystemExit('gallery-dl fallback marker not found')
    brace = s.find('{', start)
    depth = 0
    end = None
    for i in range(brace, len(s)):
        c = s[i]
        if c == '{':
            depth += 1
        elif c == '}':
            depth -= 1
            if depth == 0:
                end = i + 1
                break
    if end is None:
        raise SystemExit('could not locate end of gallery-dl fallback block')
    s = s[:end] + insert + s[end:]
    p.write_text(s, encoding='utf-8', newline='\n')
    print('HTML fallback inserted after gallery-dl')
