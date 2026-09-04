from pathlib import Path

p = Path('main.go')
s = p.read_text(encoding='utf-8')
old = '''\tstreamURL, err := a.startMegaPreview(res.Remote)\n\tif err != nil {\n\t\thttp.Error(w, err.Error(), 500)\n\t\treturn\n\t}\n\tjsonOut(w, map[string]any{\n\t\t"url":       streamURL,\n\t\t"kind":      kind,\n\t\t"streaming": true,\n\t\t"source":    "MEGA WebDAV",\n\t\t"note":      "MEGAcmd transmite numai datele cerute de player/preview. Video folosește streaming; imaginea este citită la afișare.",\n\t})\n'''
new = '''\tstreamURL, previewMode, prepareDuration, err := a.startMegaPreviewForUIV854(res.Remote)\n\tif err != nil {\n\t\thttp.Error(w, err.Error(), 500)\n\t\treturn\n\t}\n\tjsonOut(w, map[string]any{\n\t\t"url":         streamURL,\n\t\t"kind":        kind,\n\t\t"streaming":   true,\n\t\t"source":      previewMode,\n\t\t"previewMode": previewMode,\n\t\t"prepareMs":   prepareDuration.Milliseconds(),\n\t\t"note":        "Fast-path-ul UI reutilizează WebDAV-ul pregătit la scanare fără comandă MEGAcmd suplimentară. Fallback-ul per-fișier rămâne disponibil dacă nu există cache.",\n\t})\n'''
count = s.count(old)
if count != 1:
    raise SystemExit(f'expected exactly one remote preview handler block, found {count}')
s = s.replace(old, new, 1)
p.write_text(s, encoding='utf-8', newline='\n')
print('preview v8.5.4 handler patch applied; zero-command cache hits enabled')
