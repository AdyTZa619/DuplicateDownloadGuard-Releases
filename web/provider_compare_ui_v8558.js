(() => {
  'use strict';

  function rowNow() {
    try { return typeof currentRow !== 'undefined' ? currentRow : null; }
    catch (_) { return null; }
  }

  function sourceName(row = rowNow()) {
    const value = String(row?.remote?.source || 'REMOTE').trim().toUpperCase();
    return value || 'REMOTE';
  }

  function htmlEscape(value) {
    if (typeof window.esc === 'function') return window.esc(value);
    return String(value ?? '').replace(/[&<>"']/g, ch => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[ch]));
  }

  function bunkrPublicMediaURL(row) {
    if (sourceName(row) !== 'BUNKR' || !row?.remote?.handle) return '';
    try {
      const album = new URL(row.remote.url || '');
      return `${album.origin}/f/${encodeURIComponent(String(row.remote.handle))}`;
    } catch (_) {
      return '';
    }
  }

  function cyberdropPublicMediaURL(row) {
    if (sourceName(row) !== 'CYBERDROP' || !row?.remote?.providerId) return '';
    try {
      const album = new URL(row.remote.url || '');
      // gallery-dl's maintained Cyberdrop extractor uses the stable public
      // /f/<id> page and resolves a fresh signed CDN URL through Cyberdrop's API.
      // Open the selected file page, not the whole album and not an expiring CDN URL.
      return `${album.origin}/f/${encodeURIComponent(String(row.remote.providerId))}`;
    } catch (_) {
      return '';
    }
  }

  function providerRemoteMediaHTML(url, kind, name) {
    const source = sourceName();
    const ext = String(name || '').split('.').pop().toUpperCase();
    const note = '<span class="trafficNote">REMOTE • streaming la cerere</span>';
    const tag = source === 'MEGA' ? 'MEGA' : source;
    if (kind === 'image') {
      return `<img id="remoteImage" src="${url}" alt="Preview remote" onerror="remotePreviewError(this)"><span class="miniInfo">${htmlEscape(ext)} • ${htmlEscape(tag)}</span>${note}`;
    }
    if (kind === 'video') {
      return `<video id="remoteVideo" controls preload="metadata" src="${url}" onerror="remotePreviewError(this)"></video><span class="miniInfo">${htmlEscape(ext)} • ${htmlEscape(tag)} stream</span>${note}`;
    }
    if (kind === 'audio') {
      return `<audio controls preload="metadata" src="${url}" onerror="remotePreviewError(this)"></audio><span class="miniInfo">${htmlEscape(ext)} • ${htmlEscape(tag)}</span>${note}`;
    }
    return '<div class="previewEmpty">Format fără player remote integrat.</div>';
  }

  function diagnosticURL(mediaElement, row) {
    const src = String(mediaElement?.currentSrc || mediaElement?.src || '').trim();
    if (!src || !row?.id) return '';
    try {
      const u = new URL(src, window.location.href);
      u.searchParams.set('id', String(row.id));
      u.searchParams.set('diagnose', '1');
      u.searchParams.set('_ddg', String(Date.now()));
      return u.toString();
    } catch (_) {
      return '';
    }
  }

  function previewButtons() {
    return '<button class="btn primary" onclick="playRemote()">▶ Încearcă în player extern</button> <button class="btn" onclick="openRemote()">↗ Deschide sursa</button>';
  }

  function renderPreviewDiagnosis(box, data, source) {
    const ok = Boolean(data?.ok);
    const code = String(data?.code || '').trim();
    const httpStatus = Number(data?.httpStatus || 0);
    const contentType = String(data?.contentType || '').trim();
    let title = String(data?.title || '').trim();
    let detail = String(data?.detail || '').trim();

    if (ok) {
      title = 'Fișierul răspunde, dar playerul integrat nu îl poate reda';
      detail = detail || 'Sursa remote răspunde normal. Problema este probabil formatul/codec-ul sau o limitare a playerului WebView.';
    } else if (!title) {
      title = `${source}: fișier remote indisponibil`;
    }

    const meta = [];
    if (httpStatus > 0) meta.push(`HTTP ${httpStatus}`);
    if (contentType) meta.push(contentType);
    if (code) meta.push(code);
    const metaHTML = meta.length ? `<br><br><span class="miniInfo">${htmlEscape(meta.join(' • '))}</span>` : '';

    box.innerHTML = `<div class="previewEmpty"><b>${htmlEscape(title)}</b><br><br>${htmlEscape(detail)}${metaHTML}<br><br>${previewButtons()}</div>`;
  }

  async function providerRemotePreviewError(mediaElement) {
    const box = document.getElementById('remotePreview');
    if (!box) return;
    if (box.dataset.providerDiagnosing === '1') return;
    box.dataset.providerDiagnosing = '1';

    const row = rowNow();
    const source = sourceName(row);
    const diagnose = diagnosticURL(mediaElement, row);
    if (!diagnose) {
      box.innerHTML = `<div class="previewEmpty"><b>${htmlEscape(source)}: preview indisponibil</b><br><br>Nu am putut identifica URL-ul media pentru diagnostic.<br><br>${previewButtons()}</div>`;
      return;
    }

    box.innerHTML = `<div class="previewEmpty"><b>Verific problema fișierului ${htmlEscape(source)}…</b><br><br>DDG verifică doar headerele și primul byte; nu descarcă fișierul pentru acest diagnostic.</div>`;
    try {
      const response = await fetch(diagnose, {
        method: 'GET',
        headers: {'Accept':'application/json'},
        cache: 'no-store'
      });
      if (!response.ok) throw new Error(`Diagnostic HTTP ${response.status}`);
      const data = await response.json();
      renderPreviewDiagnosis(box, data, source);
    } catch (error) {
      box.innerHTML = `<div class="previewEmpty"><b>${htmlEscape(source)}: nu am putut diagnostica preview-ul</b><br><br>${htmlEscape(error?.message || String(error))}<br><br>${previewButtons()}</div>`;
    } finally {
      delete box.dataset.providerDiagnosing;
    }
  }

  async function providerOpenRemote() {
    const row = rowNow();
    if (!row?.remote) return;
    const source = sourceName(row);
    const stableMedia = bunkrPublicMediaURL(row) || cyberdropPublicMediaURL(row);
    const target = stableMedia || String(row.remote.url || '').trim();
    if (!target) return;
    try {
      await window.api('/api/open-remote', {
        method:'POST',
        headers:{'Content-Type':'application/json'},
        body:JSON.stringify({
          url: target,
          handle: source === 'MEGA' ? (row.remote.handle || '') : '',
          source
        })
      });
    } catch (error) {
      window.toast?.(error?.message || String(error));
    }
  }

  function tuneCompareUI(row = rowNow()) {
    if (!row?.remote) return;
    const source = sourceName(row);
    const open = document.getElementById('openRemote');
    if (open) {
      open.textContent = source === 'MEGA' ? '↗ MEGA' : `↗ ${source}`;
      open.title = source === 'MEGA' ? 'Deschide în MEGA' : `Deschide fișierul selectat în ${source}`;
    }

    const mediaInfo = document.getElementById('mediaInfo');
    if (mediaInfo && source !== 'MEGA' && /stream-ul MEGA/i.test(mediaInfo.textContent || '')) {
      mediaInfo.innerHTML = '<span class="muted">Apasă <b>MediaInfo</b> pentru analiza tehnică REMOTE ↔ LOCAL. Remote-ul este citit prin proxy-ul local DDG al providerului și poate transfera o cantitate mică de date pentru headere.</span>';
    }

    const detail = document.getElementById('detail');
    if (detail && source !== 'MEGA') {
      for (const label of detail.querySelectorAll('b')) {
        if (label.textContent.trim() === 'Handle remote') label.textContent = 'ID/handle provider';
      }
    }
  }

  function install() {
    window.remoteMediaHTML = providerRemoteMediaHTML;
    window.remotePreviewError = providerRemotePreviewError;
    window.openRemote = providerOpenRemote;

    const original = window.showDetail;
    if (typeof original === 'function' && !original.__providerAwareV8558) {
      const wrapped = async function(row) {
        const out = await original.apply(this, arguments);
        tuneCompareUI(row);
        setTimeout(() => tuneCompareUI(row), 0);
        return out;
      };
      wrapped.__providerAwareV8558 = true;
      window.showDetail = wrapped;
    }
    tuneCompareUI();
  }

  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', install, {once:true});
  else install();
})();
