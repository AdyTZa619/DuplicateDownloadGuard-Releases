// Generic Media Picker over CURRENT DDG results. Dedicated providers never arm
// this module; generic-site discovery owns automatic opening through events.
(() => {
  'use strict';

  let rows = [];
  let selected = new Set();
  let sourceURL = '';
  let activeFilter = 'all';
  let query = '';
  let loading = false;
  let pageIndex = 0;

  // TEST118 memory guard: never create hundreds/thousands of live image/video
  // elements at once. Chromium keeps decoder/network/buffer state for media
  // elements even when the grid is replaced, which can exhaust RAM on large
  // albums or repeated failed previews.
  const CARD_PAGE_SIZE = 72;

  const esc = value => String(value ?? '').replace(/[&<>"']/g, ch => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot',"'":'&#39;'}[ch]));

  function genericPickerAllowed(raw) {
    try {
      const u = new URL(/^https?:\/\//i.test(String(raw || '').trim()) ? String(raw || '').trim() : `https://${String(raw || '').trim()}`);
      const host = u.hostname.toLowerCase();
      if (!/^https?:$/i.test(u.protocol)) return false;
      return !(/^(?:www\.)?mega\.(?:nz|co\.nz)$/i.test(host) ||
        /(^|\.)gofile\.io$/i.test(host) || /(^|\.)erome\.com$/i.test(host) ||
        /(^|\.)cyberdrop\.[a-z]{2,}$/i.test(host) || /(^|\.)bunkr/i.test(host));
    } catch (_) { return false; }
  }

  function providerName(raw = sourceURL) {
    try {
      const host = new URL(/^https?:\/\//i.test(raw) ? raw : `https://${raw}`).hostname.toLowerCase();
      if (/(^|\.)erome\.com$/i.test(host)) return 'Erome';
      if (/(^|\.)cyberdrop\.[a-z]{2,}$/i.test(host)) return 'Cyberdrop';
      if (/(^|\.)gofile\.io$/i.test(host)) return 'GoFile';
      if (/(^|\.)bunkr/i.test(host)) return 'Bunkr';
      return host || 'Sursă online';
    } catch (_) { return 'Sursă online'; }
  }

  function mediaKind(row) {
    const remote = row?.remote || {};
    const content = String(remote.contentType || '').toLowerCase();
    if (content.startsWith('image/')) return 'image';
    if (content.startsWith('video/')) return 'video';
    if (content.startsWith('audio/')) return 'audio';
    const path = String(remote.name || remote.path || '').toLowerCase();
    const ext = path.includes('.') ? path.split('.').pop() : '';
    if (['jpg','jpeg','png','gif','webp','bmp','avif','heic','heif'].includes(ext)) return 'image';
    if (['mp4','webm','m4v','mov','mkv','avi','flv','ts','m2ts','mts','wmv'].includes(ext)) return 'video';
    if (['mp3','m4a','aac','ogg','opus','flac','wav'].includes(ext)) return 'audio';
    return 'other';
  }

  function isRecommended(row) {
    const manual = Boolean(row?.manual);
    const status = String(row?.status || row?.autoStatus || '').trim().toUpperCase();
    const guard = String(row?.guardVerdict || '').trim().toUpperCase();
    if (manual && ['HAVE','VERIFIED'].includes(status)) return false;
    if (guard === 'DUPLICATE') return false;
    if (guard === 'DOWNLOAD' && !manual) return true;
    return ['MISSING','DIFFERENT','DIFF'].includes(status);
  }

  function releaseGridMedia() {
    const grid = document.getElementById('ddgMediaPickerGridV8566');
    if (!grid) return;
    grid.querySelectorAll('video,audio').forEach(media => {
      try { media.pause(); } catch (_) {}
      try { media.removeAttribute('src'); media.load(); } catch (_) {}
    });
    grid.querySelectorAll('img').forEach(img => {
      try { img.removeAttribute('src'); } catch (_) {}
    });
  }

  function installUI() {
    if (!document.getElementById('ddgMediaPickerV8566Style')) {
      const style = document.createElement('style');
      style.id = 'ddgMediaPickerV8566Style';
      style.textContent = `
        #ddgMediaPickerV8566{position:fixed;inset:0;z-index:11800;background:rgba(3,7,12,.86);display:flex;align-items:stretch;justify-content:center;padding:18px}
        #ddgMediaPickerV8566.hidden{display:none}
        #ddgMediaPickerV8566 .shell{width:min(1500px,98vw);height:calc(100vh - 36px);background:#0d151e;border:1px solid #304256;border-radius:14px;display:flex;flex-direction:column;overflow:hidden;box-shadow:0 28px 80px rgba(0,0,0,.5)}
        #ddgMediaPickerV8566 .head{padding:13px 15px;border-bottom:1px solid #26384a;display:flex;gap:10px;align-items:center;flex-wrap:wrap}
        #ddgMediaPickerV8566 .head .title{font-weight:850;font-size:17px;margin-right:auto}
        #ddgMediaPickerV8566 .tools{padding:10px 15px;border-bottom:1px solid #223244;display:flex;gap:8px;align-items:center;flex-wrap:wrap}
        #ddgMediaPickerV8566 .search{min-width:240px;max-width:420px}
        #ddgMediaPickerV8566 .summary{color:#9fb4ca;font-size:12px;margin-left:auto}
        #ddgMediaPickerV8566 .grid{padding:14px;overflow:auto;display:grid;grid-template-columns:repeat(auto-fill,minmax(190px,1fr));gap:10px;align-content:start;flex:1}
        #ddgMediaPickerV8566 .mediaCard{border:1px solid #26394b;background:#0a1118;border-radius:10px;overflow:hidden;cursor:pointer;min-width:0;position:relative}
        #ddgMediaPickerV8566 .mediaCard:hover{border-color:#46627d} #ddgMediaPickerV8566 .mediaCard.on{border-color:#4da3ff;box-shadow:inset 0 0 0 1px #4da3ff}
        #ddgMediaPickerV8566 .thumb{height:170px;background:#05090d;display:flex;align-items:center;justify-content:center;overflow:hidden;position:relative}
        #ddgMediaPickerV8566 .thumb img,#ddgMediaPickerV8566 .thumb video{width:100%;height:100%;object-fit:contain;background:#05090d}
        #ddgMediaPickerV8566 .fallback{color:#8ea4bb;text-align:center;padding:20px;font-weight:800}
        #ddgMediaPickerV8566 .type{position:absolute;top:7px;left:7px;background:rgba(0,0,0,.72);border:1px solid #44586d;border-radius:999px;padding:2px 6px;font-size:10px}
        #ddgMediaPickerV8566 .pick{position:absolute;top:7px;right:7px;width:23px;height:23px;border-radius:7px;background:rgba(0,0,0,.74);border:1px solid #5d7186;display:grid;place-items:center;font-size:13px}
        #ddgMediaPickerV8566 .mediaCard.on .pick{background:#4da3ff;color:#07121d;border-color:#4da3ff}
        #ddgMediaPickerV8566 .meta{padding:8px 9px;min-width:0}.name{white-space:nowrap;overflow:hidden;text-overflow:ellipsis;font-size:12px;font-weight:750}.sub{white-space:nowrap;overflow:hidden;text-overflow:ellipsis;color:#8398ae;font-size:10px;margin-top:4px}
        #ddgMediaPickerV8566 .foot{padding:11px 15px;border-top:1px solid #26384a;display:flex;gap:8px;align-items:center;flex-wrap:wrap} #ddgMediaPickerV8566 .foot .count{margin-right:auto;font-weight:800}
        #ddgMediaPickerLaunchV8566{margin-top:8px}
        @media(max-width:700px){#ddgMediaPickerV8566{padding:6px}#ddgMediaPickerV8566 .shell{height:calc(100vh - 12px)}#ddgMediaPickerV8566 .grid{grid-template-columns:repeat(2,minmax(0,1fr));padding:8px}#ddgMediaPickerV8566 .thumb{height:135px}}
      `;
      document.head.appendChild(style);
    }
    if (!document.getElementById('ddgMediaPickerV8566')) {
      document.body.insertAdjacentHTML('beforeend', `
        <div id="ddgMediaPickerV8566" class="hidden" role="dialog" aria-modal="true"><div class="shell">
          <div class="head"><div class="title" id="ddgMediaPickerTitleV8566">Media Picker</div><button class="btn" id="ddgMediaPickerRefreshV8566">↻ Reîncarcă</button><button class="btn" id="ddgMediaPickerCloseV8566">Închide</button></div>
          <div class="tools"><input class="field search" id="ddgMediaPickerSearchV8566" placeholder="Caută nume / cale…"><button class="chip on" data-picker-filter="all">Toate</button><button class="chip" data-picker-filter="image">Imagini</button><button class="chip" data-picker-filter="video">Video</button><button class="chip" data-picker-filter="audio">Audio</button><button class="btn" id="ddgMediaPickerPrevV8566">‹</button><span id="ddgMediaPickerPageV8566" class="small muted">1/1</span><button class="btn" id="ddgMediaPickerNextV8566">›</button><span class="summary" id="ddgMediaPickerSummaryV8566">—</span></div>
          <div class="grid" id="ddgMediaPickerGridV8566"></div>
          <div class="foot"><span class="count" id="ddgMediaPickerCountV8566">0 selectate</span><button class="btn" id="ddgMediaPickerClearV8566">Nimic</button><button class="btn" id="ddgMediaPickerRecommendedV8566">Selectează lipsurile</button><button class="btn" id="ddgMediaPickerAllVisibleV8566">Selectează toate afișate</button><button class="btn primary" id="ddgMediaPickerSendSelectedV8566">Trimite lipsurile selectate în JD</button></div>
        </div></div>`);
      document.getElementById('ddgMediaPickerCloseV8566')?.addEventListener('click', close);
      document.getElementById('ddgMediaPickerRefreshV8566')?.addEventListener('click', () => loadRows(true));
      document.getElementById('ddgMediaPickerClearV8566')?.addEventListener('click', () => { selected.clear(); render(); });
      document.getElementById('ddgMediaPickerRecommendedV8566')?.addEventListener('click', () => { for (const row of rows) if (isRecommended(row)) selected.add(Number(row.id)); render(); });
      document.getElementById('ddgMediaPickerAllVisibleV8566')?.addEventListener('click', () => { for (const row of visibleRows()) selected.add(Number(row.id)); render(); });
      document.getElementById('ddgMediaPickerSendSelectedV8566')?.addEventListener('click', () => sendIDs([...selected]));
      document.getElementById('ddgMediaPickerPrevV8566')?.addEventListener('click', () => { if (pageIndex > 0) { pageIndex--; render(); } });
      document.getElementById('ddgMediaPickerNextV8566')?.addEventListener('click', () => { const pages = Math.max(1, Math.ceil(filteredRows().length / CARD_PAGE_SIZE)); if (pageIndex + 1 < pages) { pageIndex++; render(); } });
      document.getElementById('ddgMediaPickerSearchV8566')?.addEventListener('input', event => { query = String(event.target.value || '').trim().toLowerCase(); pageIndex = 0; render(); });
      document.querySelectorAll('#ddgMediaPickerV8566 [data-picker-filter]').forEach(button => button.addEventListener('click', () => { activeFilter = button.dataset.pickerFilter || 'all'; pageIndex = 0; document.querySelectorAll('#ddgMediaPickerV8566 [data-picker-filter]').forEach(x => x.classList.toggle('on', x === button)); render(); }));
      document.getElementById('ddgMediaPickerV8566')?.addEventListener('click', event => { if (event.target?.id === 'ddgMediaPickerV8566') close(); });
    }
    installLaunchButton();
  }

  function installLaunchButton() {
    if (document.getElementById('ddgMediaPickerLaunchV8566')) return;
    const input = document.getElementById('directUrl');
    const body = input?.closest('.sectionBody');
    if (!body) return;
    const button = document.createElement('button');
    button.id = 'ddgMediaPickerLaunchV8566'; button.className = 'btn'; button.type = 'button'; button.textContent = '▦ Media Picker';
    button.title = 'Vizualizează și selectează rezultatele scanării curente, fără rescanare HDD.';
    button.addEventListener('click', () => open(String(input?.value || '')));
    const state = document.getElementById('universalProviderState');
    if (state?.parentElement) state.insertAdjacentElement('afterend', button); else body.appendChild(button);
  }

  async function fetchAllRows() {
    const out = []; let offset = 0;
    for (let page = 0; page < 500; page++) {
      const data = await window.api(`/api/results?status=ALL&q=&offset=${offset}&limit=1000&sort=path&order=asc`);
      const batch = Array.isArray(data?.rows) ? data.rows : [];
      out.push(...batch);
      if (!batch.length || offset + batch.length >= Number(data?.total || 0)) break;
      offset += batch.length;
    }
    return out;
  }

  async function loadRows(force = false) {
    if (loading) return;
    if (!force && rows.length) return render();
    loading = true;
    releaseGridMedia();
    const grid = document.getElementById('ddgMediaPickerGridV8566');
    if (grid) grid.innerHTML = '<div class="previewLoading"><div class="spin"></div><b>Citesc rezultatele DDG curente…</b><span class="small">Fără rescanare HDD.</span></div>';
    try {
      rows = await fetchAllRows();
      const valid = new Set(rows.map(r => Number(r.id)));
      selected = new Set([...selected].filter(id => valid.has(id)));
      pageIndex = 0;
      render();
    } catch (error) { if (grid) grid.innerHTML = `<div class="previewEmpty">${esc(error?.message || String(error))}</div>`; }
    finally { loading = false; }
  }

  function filteredRows() {
    return rows.filter(row => {
      if (activeFilter !== 'all' && mediaKind(row) !== activeFilter) return false;
      if (!query) return true;
      return `${row?.remote?.name || ''} ${row?.remote?.path || ''} ${row?.remote?.source || ''}`.toLowerCase().includes(query);
    });
  }

  function visibleRows() {
    const shown = filteredRows();
    const pages = Math.max(1, Math.ceil(shown.length / CARD_PAGE_SIZE));
    if (pageIndex >= pages) pageIndex = pages - 1;
    const start = pageIndex * CARD_PAGE_SIZE;
    return shown.slice(start, start + CARD_PAGE_SIZE);
  }

  function previewHTML(row) {
    const kind = mediaKind(row);
    const url = `/api/provider-preview/media?id=${encodeURIComponent(String(row.id))}`;
    if (kind === 'image') return `<img loading="lazy" decoding="async" src="${url}" alt="${esc(row?.remote?.name || '')}">`;
    // preload=none is deliberate: creating a card must not open a network/media
    // decoder for every video. Chromium starts it only when the user interacts.
    if (kind === 'video') return `<video muted controls preload="none" src="${url}"></video>`;
    if (kind === 'audio') return '<div class="fallback">AUDIO<br><small>selectează pentru JD</small></div>';
    return '<div class="fallback">FIȘIER<br><small>fără preview</small></div>';
  }

  function render() {
    const grid = document.getElementById('ddgMediaPickerGridV8566');
    if (!grid || loading) return;
    const shown = filteredRows();
    const pages = Math.max(1, Math.ceil(shown.length / CARD_PAGE_SIZE));
    if (pageIndex >= pages) pageIndex = pages - 1;
    const pageRows = visibleRows();
    const start = shown.length ? pageIndex * CARD_PAGE_SIZE + 1 : 0;
    const end = shown.length ? Math.min(shown.length, (pageIndex + 1) * CARD_PAGE_SIZE) : 0;
    document.getElementById('ddgMediaPickerSummaryV8566').textContent = `${shown.length.toLocaleString('ro-RO')} rezultate • ${start}-${end} în memorie vizuală • ${rows.length.toLocaleString('ro-RO')} total`;
    document.getElementById('ddgMediaPickerPageV8566').textContent = `${pageIndex + 1}/${pages}`;
    document.getElementById('ddgMediaPickerPrevV8566').disabled = pageIndex <= 0;
    document.getElementById('ddgMediaPickerNextV8566').disabled = pageIndex + 1 >= pages;
    document.getElementById('ddgMediaPickerCountV8566').textContent = `${selected.size.toLocaleString('ro-RO')} selectate`;
    document.getElementById('ddgMediaPickerSendSelectedV8566').disabled = selected.size === 0;
    document.getElementById('ddgMediaPickerAllVisibleV8566').disabled = pageRows.length === 0;
    releaseGridMedia();
    if (!shown.length) { grid.innerHTML = '<div class="previewEmpty">Nu există rezultate pentru filtrul curent.</div>'; return; }
    grid.innerHTML = pageRows.map(row => {
      const id = Number(row.id), kind = mediaKind(row), on = selected.has(id);
      const name = row?.remote?.name || row?.remote?.path || `#${id}`;
      const status = String(row?.status || '').toUpperCase();
      return `<div class="mediaCard ${on?'on':''}" data-picker-id="${id}" title="${esc(row?.remote?.path || '')}"><div class="thumb">${previewHTML(row)}<span class="type">${esc(kind.toUpperCase())}</span><span class="pick">${on?'✓':''}</span></div><div class="meta"><div class="name">${esc(name)}</div><div class="sub">${esc(status)}${row?.remote?.size > 0 ? ` • ${window.fmt ? window.fmt(row.remote.size) : row.remote.size}` : ''}</div></div></div>`;
    }).join('');
    grid.scrollTop = 0;
    grid.querySelectorAll('.mediaCard').forEach(card => card.addEventListener('click', event => {
      if (event.target?.closest?.('video,audio,button,input')) return;
      const id = Number(card.dataset.pickerId); if (!Number.isFinite(id)) return;
      selected.has(id) ? selected.delete(id) : selected.add(id); render();
    }));
  }

  async function sendIDs(ids) {
    const unique = [...new Set((ids || []).map(Number).filter(Number.isFinite))];
    if (!unique.length) return window.toast?.('Nu există fișiere de trimis');
    const jd = window.ddgJDownloaderFastV8566;
    if (!jd?.sendExactIDs) return window.toast?.('Modulul JDownloader rapid nu este încă încărcat.');
    try { await jd.sendExactIDs(unique, {confirm:true}); } catch (_) {}
  }

  async function open(raw = '') {
    installUI();
    sourceURL = String(raw || document.getElementById('directUrl')?.value || '').trim();
    if (!genericPickerAllowed(sourceURL)) {
      window.toast?.('Media Picker este disponibil numai pentru site-uri generice; acest link folosește providerul dedicat.');
      return;
    }
    releaseGridMedia();
    rows = []; selected.clear(); activeFilter = 'all'; query = ''; pageIndex = 0;
    const search = document.getElementById('ddgMediaPickerSearchV8566'); if (search) search.value = '';
    document.querySelectorAll('#ddgMediaPickerV8566 [data-picker-filter]').forEach(x => x.classList.toggle('on', x.dataset.pickerFilter === 'all'));
    document.getElementById('ddgMediaPickerTitleV8566').textContent = `Media Picker — ${providerName(sourceURL)}`;
    document.getElementById('ddgMediaPickerV8566')?.classList.remove('hidden');
    await loadRows(true);
  }

  function close() {
    releaseGridMedia();
    const grid = document.getElementById('ddgMediaPickerGridV8566');
    if (grid) grid.innerHTML = '';
    document.getElementById('ddgMediaPickerV8566')?.classList.add('hidden');
  }

  function boot() {
    installUI();
    setTimeout(installLaunchButton, 500); setTimeout(installLaunchButton, 1500);
  }

  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', boot, {once:true}); else boot();
  window.ddgMediaPickerV8566 = {open, close, reload:() => loadRows(true), genericPickerAllowed};
})();
