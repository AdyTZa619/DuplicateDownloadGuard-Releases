from pathlib import Path

# Backend: allow the browser to explicitly bypass the optimistic root fast path
# after a real media load error.
p = Path('main.go')
s = p.read_text(encoding='utf-8')
old = '''func (a *App) handleRemotePreviewStart(w http.ResponseWriter, r *http.Request) {\n\tvar req struct {\n\t\tID int `json:"id"`\n\t}\n'''
new = '''func (a *App) handleRemotePreviewStart(w http.ResponseWriter, r *http.Request) {\n\tvar req struct {\n\t\tID            int  `json:"id"`\n\t\tForceFallback bool `json:"forceFallback,omitempty"`\n\t}\n'''
if s.count(old) != 1:
    raise SystemExit(f'preview request block count={s.count(old)}')
s = s.replace(old, new, 1)
old_call = 'streamURL, previewMode, prepareDuration, err := a.startMegaPreviewForUIV854(res.Remote)'
new_call = 'streamURL, previewMode, prepareDuration, err := a.startMegaPreviewForUIV854(res.Remote, req.ForceFallback)'
if s.count(old_call) != 1:
    raise SystemExit(f'preview fast call count={s.count(old_call)}')
s = s.replace(old_call, new_call, 1)
p.write_text(s, encoding='utf-8', newline='\n')

p = Path('web/exact_guard.js')
s = p.read_text(encoding='utf-8')
old_delay = '      }, 320);'
if s.count(old_delay) != 1:
    raise SystemExit(f'preview debounce count={s.count(old_delay)}')
s = s.replace(old_delay, '      }, 140);', 1)
marker = '// Operation HUD v8.5.1 — immersive live status in the top-right corner.'
if s.count(marker) != 1:
    raise SystemExit('operation HUD marker missing/ambiguous')
block = r'''// MEGA fast-root recovery v8.5.4. Root URLs are returned optimistically for
// speed. If the browser reports a real media error, retry once through the
// proven per-file WebDAV path instead of leaving the user at a dead preview.
(() => {
  'use strict';
  if (typeof window.remotePreviewError !== 'function') return;
  const originalRemotePreviewErrorV854 = window.remotePreviewError;
  let lastFallbackID = 0;
  let lastFallbackAt = 0;

  window.remotePreviewError = async function () {
    const row = typeof currentRow !== 'undefined' ? currentRow : null;
    const id = Number(row?.id || 0);
    const sourceEl = document.getElementById('remoteSource');
    const sourceText = String(sourceEl?.textContent || '').toUpperCase();
    const now = Date.now();
    const recentlyRetried = id && lastFallbackID === id && now - lastFallbackAt < 15000;
    if (id && sourceText.includes('MEGA FAST ROOT') && !recentlyRetried) {
      lastFallbackID = id;
      lastFallbackAt = now;
      const preview = document.getElementById('remotePreview');
      if (preview) preview.innerHTML = '<div class="previewLoading"><div class="spin"></div><b>Fast preview nu a răspuns; comut pe fallback MEGA…</b><span class="small">O singură încercare WebDAV per-fișier.</span></div>';
      try {
        const d = await api('/api/remote-preview/start', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ id, forceFallback: true })
        });
        if (!currentRow || Number(currentRow.id) !== id) return;
        const kind = d.kind || previewKind(row.remote?.name || row.remote?.path || '');
        remotePreviewActive = true;
        const stop = document.getElementById('stopRemote');
        if (stop) stop.disabled = false;
        if (sourceEl) {
          sourceEl.textContent = `${d.source || 'MEGA FALLBACK'}${Number.isFinite(Number(d.prepareMs)) ? ` • ${Number(d.prepareMs)} ms` : ''} • LIVE`;
          sourceEl.classList.add('remoteLive');
        }
        if (preview) preview.innerHTML = remoteMediaHTML(d.url, kind, row.remote?.name || row.remote?.path || 'remote');
        return;
      } catch (error) {
        if (preview) preview.innerHTML = `<div class="previewEmpty"><b>Fallback MEGA eșuat.</b><br>${esc(error.message)}<br><br><button class="btn primary" onclick="playRemote()">▶ Încearcă în player extern</button> <button class="btn" onclick="openRemote()">↗ MEGA</button></div>`;
        return;
      }
    }
    return originalRemotePreviewErrorV854();
  };
})();

'''
s = s.replace(marker, block + marker, 1)
p.write_text(s, encoding='utf-8', newline='\n')
print('preview v8.5.4 fallback + debounce patch applied; validation triggered')
