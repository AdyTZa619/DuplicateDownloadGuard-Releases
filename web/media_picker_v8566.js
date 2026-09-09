// Generic Media Picker with two explicit stages:
// 1) discover and choose real page media; 2) compare only that choice with PC.
(() => {
  'use strict';

  let rows = [];
  let selected = new Set();
  let sourceURL = '';
  let activeFilter = 'all';
  let query = '';
  let loading = false;
  let pageIndex = 0;
  let pickerMode = 'results';
  let discoveryWarnings = [];
  const CARD_PAGE_SIZE = 72;

  const esc = value => String(value ?? '').replace(/[&<>"']/g, ch => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[ch]));

  function genericPickerAllowed(raw) {
    try {
      const value = String(raw || '').trim();
      const u = new URL(/^https?:\/\//i.test(value) ? value : `https://${value}`);
      const host = u.hostname.toLowerCase();
      if (!/^https?:$/i.test(u.protocol)) return false;
      return !(/^(?:www\.)?mega\.(?:nz|co\.nz)$/i.test(host) ||
        /(^|\.)gofile\.io$/i.test(host) || /(^|\.)erome\.com$/i.test(host) ||
        /(^|\.)cyberdrop\.[a-z]{2,}$/i.test(host) || /(^|\.)bunkr/i.test(host));
    } catch (_) { return false; }
  }

  function providerName(raw = sourceURL) {
    try { return new URL(/^https?:\/\//i.test(raw) ? raw : `https://${raw}`).hostname.toLowerCase() || 'Sursă online'; }
    catch (_) { return 'Sursă online'; }
  }

  function rowKey(row) {
    return pickerMode === 'discovery' ? String(row?._pickerKey || '') : `result:${Number(row?.id)}`;
  }

  function mediaKind(row) {
    if (pickerMode === 'discovery') {
      const kind = String(row?.kind || '').toLowerCase();
      return ['hls', 'dash', 'video'].includes(kind) ? 'video' : (['image', 'audio'].includes(kind) ? kind : 'other');
    }
    const remote = row?.remote || {};
    const content = String(remote.contentType || '').toLowerCase();
    if (content.startsWith('image/')) return 'image';
    if (content.startsWith('video/')) return 'video';
    if (content.startsWith('audio/')) return 'audio';
    const path = String(remote.name || remote.path || '').toLowerCase();
    const ext = path.includes('.') ? path.split('.').pop() : '';
    if (['jpg','jpeg','png','gif','webp','bmp','avif','heic','heif'].includes(ext)) return 'image';
    if (['mp4','webm','m4v','mov','mkv','avi','flv','ts','m2ts','mts','wmv','m3u8','mpd'].includes(ext)) return 'video';
    if (['mp3','m4a','aac','ogg','opus','flac','wav'].includes(ext)) return 'audio';
    return 'other';
  }

  function typeLabel(row) {
    if (pickerMode === 'discovery') return String(row?.kind || 'media').toUpperCase();
    return mediaKind(row).toUpperCase();
  }

  function isRecommended(row) {
    if (pickerMode === 'discovery') return ['video', 'hls', 'dash', 'audio'].includes(String(row?.kind || '').toLowerCase());
    const manual = Boolean(row?.manual);
    const status = String(row?.status || row?.autoStatus || '').trim().toUpperCase();
    const guard = String(row?.guardVerdict || '').trim().toUpperCase();
    if (guard === 'DUPLICATE') return false;
    if (guard === 'DOWNLOAD') return true;
    if (manual && ['HAVE','VERIFIED'].includes(status)) return false;
    return ['MISSING','DIFFERENT','DIFF'].includes(status);
  }

  function releaseGridMedia() {
    const grid = document.getElementById('ddgMediaPickerGridV8566');
    if (!grid) return;
    grid.querySelectorAll('video,audio').forEach(media => {
      try { media.pause(); } catch (_) {}
      try { media.removeAttribute('src'); media.load(); } catch (_) {}
    });
    grid.querySelectorAll('img').forEach(img => { try { img.removeAttribute('src'); } catch (_) {} });
  }

  function installUI() {
    if (!document.getElementById('ddgMediaPickerV8566Style')) {
      const style = document.createElement('style');
      style.id = 'ddgMediaPickerV8566Style';
      style.textContent = `
        #ddgMediaPickerV8566{position:fixed;inset:0;z-index:11800;background:rgba(3,7,12,.88);display:flex;align-items:stretch;justify-content:center;padding:18px}
        #ddgMediaPickerV8566.hidden{display:none}#ddgMediaPickerV8566 .shell{width:min(1500px,98vw);height:calc(100vh - 36px);background:#0d151e;border:1px solid #304256;border-radius:14px;display:flex;flex-direction:column;overflow:hidden;box-shadow:0 28px 80px rgba(0,0,0,.5)}
        #ddgMediaPickerV8566 .head{padding:13px 15px;border-bottom:1px solid #26384a;display:flex;gap:10px;align-items:center;flex-wrap:wrap}#ddgMediaPickerV8566 .head .title{font-weight:850;font-size:17px;margin-right:auto}
        #ddgMediaPickerV8566 .stage{padding:8px 15px;background:#101e2b;color:#bcd4eb;font-size:12px;border-bottom:1px solid #26384a}#ddgMediaPickerV8566 .stage b{color:#64b5ff}
        #ddgMediaPickerV8566 .tools{padding:10px 15px;border-bottom:1px solid #223244;display:flex;gap:8px;align-items:center;flex-wrap:wrap}#ddgMediaPickerV8566 .search{min-width:240px;max-width:420px}#ddgMediaPickerV8566 .summary{color:#9fb4ca;font-size:12px;margin-left:auto}
        #ddgMediaPickerV8566 .grid{padding:14px;overflow:auto;display:grid;grid-template-columns:repeat(auto-fill,minmax(220px,1fr));gap:10px;align-content:start;flex:1}
        #ddgMediaPickerV8566 .mediaCard{border:1px solid #26394b;background:#0a1118;border-radius:10px;overflow:hidden;cursor:pointer;min-width:0;position:relative}#ddgMediaPickerV8566 .mediaCard:hover{border-color:#46627d}#ddgMediaPickerV8566 .mediaCard.on{border-color:#4da3ff;box-shadow:inset 0 0 0 1px #4da3ff}
        #ddgMediaPickerV8566 .thumb{height:175px;background:#05090d;display:flex;align-items:center;justify-content:center;overflow:hidden;position:relative}#ddgMediaPickerV8566 .thumb img,#ddgMediaPickerV8566 .thumb video{width:100%;height:100%;object-fit:contain;background:#05090d}#ddgMediaPickerV8566 .thumb audio{width:90%}
        #ddgMediaPickerV8566 .fallback{color:#8ea4bb;text-align:center;padding:20px;font-weight:800}#ddgMediaPickerV8566 .type{position:absolute;top:7px;left:7px;background:rgba(0,0,0,.78);border:1px solid #44586d;border-radius:999px;padding:2px 6px;font-size:10px}#ddgMediaPickerV8566 .pick{position:absolute;top:7px;right:7px;width:23px;height:23px;border-radius:7px;background:rgba(0,0,0,.78);border:1px solid #5d7186;display:grid;place-items:center;font-size:13px}#ddgMediaPickerV8566 .mediaCard.on .pick{background:#4da3ff;color:#07121d;border-color:#4da3ff}
        #ddgMediaPickerV8566 .meta{padding:8px 9px;min-width:0}#ddgMediaPickerV8566 .name{white-space:nowrap;overflow:hidden;text-overflow:ellipsis;font-size:12px;font-weight:750}#ddgMediaPickerV8566 .sub{white-space:nowrap;overflow:hidden;text-overflow:ellipsis;color:#8398ae;font-size:10px;margin-top:4px}#ddgMediaPickerV8566 .qualities{display:flex;gap:4px;flex-wrap:wrap;margin-top:7px;max-height:46px;overflow:hidden}#ddgMediaPickerV8566 .quality{font-size:9px;padding:2px 5px;border:1px solid #344b61;border-radius:999px;color:#b6cde3;background:#101c27}#ddgMediaPickerV8566 .openSource{display:inline-block;margin-top:7px;color:#64b5ff;font-size:10px;text-decoration:none}#ddgMediaPickerV8566 .openSource:hover{text-decoration:underline}
        #ddgMediaPickerV8566 .foot{padding:11px 15px;border-top:1px solid #26384a;display:flex;gap:8px;align-items:center;flex-wrap:wrap}#ddgMediaPickerV8566 .foot .count{margin-right:auto;font-weight:800}#ddgMediaPickerLaunchV8566{margin-top:8px}
        @media(max-width:700px){#ddgMediaPickerV8566{padding:6px}#ddgMediaPickerV8566 .shell{height:calc(100vh - 12px)}#ddgMediaPickerV8566 .grid{grid-template-columns:repeat(2,minmax(0,1fr));padding:8px}#ddgMediaPickerV8566 .thumb{height:135px}}
      `;
      document.head.appendChild(style);
    }
    if (!document.getElementById('ddgMediaPickerV8566')) {
      document.body.insertAdjacentHTML('beforeend', `
        <div id="ddgMediaPickerV8566" class="hidden" role="dialog" aria-modal="true"><div class="shell">
          <div class="head"><div class="title" id="ddgMediaPickerTitleV8566">Media Picker</div><button class="btn" id="ddgMediaPickerRefreshV8566">↻ Reîncarcă</button><button class="btn" id="ddgMediaPickerCloseV8566">Închide</button></div>
          <div class="stage" id="ddgMediaPickerStageV8566"></div>
          <div class="tools"><input class="field search" id="ddgMediaPickerSearchV8566" placeholder="Caută titlu / sursă / calitate…"><button class="chip on" data-picker-filter="all">Toate</button><button class="chip" data-picker-filter="image">Imagini</button><button class="chip" data-picker-filter="video">Video</button><button class="chip" data-picker-filter="audio">Audio</button><button class="btn" id="ddgMediaPickerPrevV8566">‹</button><span id="ddgMediaPickerPageV8566" class="small muted">1/1</span><button class="btn" id="ddgMediaPickerNextV8566">›</button><span class="summary" id="ddgMediaPickerSummaryV8566">—</span></div>
          <div class="grid" id="ddgMediaPickerGridV8566"></div>
          <div class="foot"><span class="count" id="ddgMediaPickerCountV8566">0 selectate</span><button class="btn" id="ddgMediaPickerClearV8566">Nimic</button><button class="btn" id="ddgMediaPickerRecommendedV8566">Selectează video</button><button class="btn" id="ddgMediaPickerAllVisibleV8566">Selectează toate afișate</button><button class="btn primary" id="ddgMediaPickerSendSelectedV8566">Compară selectatele cu PC</button></div>
        </div></div>`);
      document.getElementById('ddgMediaPickerCloseV8566')?.addEventListener('click', close);
      document.getElementById('ddgMediaPickerRefreshV8566')?.addEventListener('click', refresh);
      document.getElementById('ddgMediaPickerClearV8566')?.addEventListener('click', () => { selected.clear(); render(); });
      document.getElementById('ddgMediaPickerRecommendedV8566')?.addEventListener('click', () => { for (const row of rows) if (isRecommended(row)) selected.add(rowKey(row)); render(); });
      document.getElementById('ddgMediaPickerAllVisibleV8566')?.addEventListener('click', () => { for (const row of visibleRows()) selected.add(rowKey(row)); render(); });
      document.getElementById('ddgMediaPickerSendSelectedV8566')?.addEventListener('click', primaryAction);
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
    button.title = 'Detectează media paginii, apoi compară numai selecția ta cu PC-ul.';
    button.addEventListener('click', () => {
      const raw = String(input?.value || '').trim();
      if (!genericPickerAllowed(raw)) return window.toast?.('Media Picker este disponibil numai pentru site-uri generice.');
      if (window.ddgGenericMediaDiscoveryV85127?.run) void window.ddgGenericMediaDiscoveryV85127.run(true);
      else void open(raw);
    });
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
    if (loading || pickerMode !== 'results') return;
    if (!force && rows.length) return render();
    loading = true;
    showLoading('Citesc rezultatele comparației…', 'Fără rescanare HDD.');
    try {
      rows = await fetchAllRows();
      const valid = new Set(rows.map(row => `result:${Number(row.id)}`));
      selected = new Set([...selected].filter(key => valid.has(key)));
      pageIndex = 0;
    } catch (error) {
      showError(error);
    } finally {
      loading = false;
      render();
    }
  }

  function searchText(row) {
    if (pickerMode === 'discovery') return `${row?.title || ''} ${row?.url || ''} ${row?.page || ''} ${row?.via || ''} ${row?.extractor || ''} ${(row?.qualities || []).map(x => x?.label || '').join(' ')}`.toLowerCase();
    return `${row?.remote?.name || ''} ${row?.remote?.path || ''} ${row?.remote?.source || ''}`.toLowerCase();
  }

  function filteredRows() {
    return rows.filter(row => (activeFilter === 'all' || mediaKind(row) === activeFilter) && (!query || searchText(row).includes(query)));
  }

  function visibleRows() {
    const shown = filteredRows();
    const pages = Math.max(1, Math.ceil(shown.length / CARD_PAGE_SIZE));
    if (pageIndex >= pages) pageIndex = pages - 1;
    return shown.slice(pageIndex * CARD_PAGE_SIZE, (pageIndex + 1) * CARD_PAGE_SIZE);
  }

  function discoveryAssetURL(row, asset) {
    if (asset === 'thumbnail' && !String(row?.thumbnail || '').trim()) return '';
    if (asset === 'media' && !String(row?.previewUrl || '').trim()) return '';
    if (row?.token) return `/api/generic-media/preview?token=${encodeURIComponent(String(row.token))}&asset=${asset}`;
    return asset === 'thumbnail' ? String(row?.thumbnail || '') : String(row?.previewUrl || '');
  }

  function previewHTML(row) {
    const kind = mediaKind(row);
    if (pickerMode === 'results') {
      const url = `/api/provider-preview/media?id=${encodeURIComponent(String(row.id))}`;
      if (kind === 'image') return `<img loading="lazy" decoding="async" src="${url}" alt="${esc(row?.remote?.name || '')}">`;
      if (kind === 'video') return `<video muted controls preload="none" src="${url}"></video>`;
      if (kind === 'audio') return '<div class="fallback">AUDIO<br><small>selectează pentru JD</small></div>';
      return '<div class="fallback">FIȘIER<br><small>fără preview</small></div>';
    }
    const media = discoveryAssetURL(row, 'media');
    const thumbnail = discoveryAssetURL(row, 'thumbnail');
    if (kind === 'image' && media) return `<img loading="lazy" decoding="async" src="${esc(media)}" alt="${esc(row?.title || '')}">`;
    if (kind === 'video' && media) return `<video muted controls preload="none"${thumbnail ? ` poster="${esc(thumbnail)}"` : ''} src="${esc(media)}"></video>`;
    if (kind === 'audio' && media) return `<audio controls preload="none" src="${esc(media)}"></audio>`;
    if (thumbnail) return `<img loading="lazy" decoding="async" src="${esc(thumbnail)}" alt="${esc(row?.title || '')}">`;
    return '<div class="fallback">SURSĂ WEB<br><small>va fi extrasă la comparare</small></div>';
  }

  function qualitiesHTML(row) {
    if (pickerMode !== 'discovery') return '';
    const qualities = Array.isArray(row?.qualities) ? row.qualities : [];
    if (!qualities.length) return '<div class="qualities"><span class="quality">calitate automată</span></div>';
    const shown = qualities.slice(0, 5).map(q => `<span class="quality" title="format ${esc(q?.formatId || '?')}">${esc(q?.label || `${q?.height || '?'}p`)}</span>`).join('');
    return `<div class="qualities">${shown}${qualities.length > 5 ? `<span class="quality">+${qualities.length - 5}</span>` : ''}</div>`;
  }

  function durationLabel(seconds) {
    seconds = Math.round(Number(seconds || 0));
    if (!seconds) return '';
    const minutes = Math.floor(seconds / 60);
    return `${minutes}:${String(seconds % 60).padStart(2, '0')}`;
  }

  function render() {
    const grid = document.getElementById('ddgMediaPickerGridV8566');
    if (!grid || loading) return;
    syncChrome();
    const shown = filteredRows();
    const pages = Math.max(1, Math.ceil(shown.length / CARD_PAGE_SIZE));
    if (pageIndex >= pages) pageIndex = pages - 1;
    const pageRows = visibleRows();
    const start = shown.length ? pageIndex * CARD_PAGE_SIZE + 1 : 0;
    const end = shown.length ? Math.min(shown.length, (pageIndex + 1) * CARD_PAGE_SIZE) : 0;
    const totals = rows.reduce((out, row) => { const kind = mediaKind(row); out[kind] = (out[kind] || 0) + 1; return out; }, {});
    const breakdown = [`Video ${Number(totals.video || 0).toLocaleString('ro-RO')}`, `Imagini ${Number(totals.image || 0).toLocaleString('ro-RO')}`, `Audio ${Number(totals.audio || 0).toLocaleString('ro-RO')}`];
    document.getElementById('ddgMediaPickerSummaryV8566').textContent = `${shown.length.toLocaleString('ro-RO')} afișate • ${start}-${end} • ${breakdown.join(' • ')}`;
    document.getElementById('ddgMediaPickerPageV8566').textContent = `${pageIndex + 1}/${pages}`;
    document.getElementById('ddgMediaPickerPrevV8566').disabled = pageIndex <= 0;
    document.getElementById('ddgMediaPickerNextV8566').disabled = pageIndex + 1 >= pages;
    document.getElementById('ddgMediaPickerCountV8566').textContent = `${selected.size.toLocaleString('ro-RO')} selectate`;
    document.getElementById('ddgMediaPickerSendSelectedV8566').disabled = selected.size === 0;
    document.getElementById('ddgMediaPickerAllVisibleV8566').disabled = pageRows.length === 0;
    releaseGridMedia();
    if (!shown.length) { grid.innerHTML = '<div class="previewEmpty">Nu există rezultate pentru filtrul curent.</div>'; return; }
    grid.innerHTML = pageRows.map(row => {
      const key = rowKey(row), on = selected.has(key);
      const name = pickerMode === 'discovery' ? (row?.title || row?.url || 'Media fără titlu') : (row?.remote?.name || row?.remote?.path || `#${row?.id}`);
      const sub = pickerMode === 'discovery'
        ? [row?.via || 'web', row?.extractor, durationLabel(row?.duration), row?.height ? `${row.height}p` : ''].filter(Boolean).join(' • ')
        : [String(row?.status || '').toUpperCase(), row?.remote?.size > 0 ? (window.fmt ? window.fmt(row.remote.size) : row.remote.size) : ''].filter(Boolean).join(' • ');
      const sourceLink = pickerMode === 'discovery' && /^https?:\/\//i.test(String(row?.url || '')) ? `<a class="openSource" href="${esc(row.url)}" target="_blank" rel="noopener noreferrer">Deschide sursa ↗</a>` : '';
      return `<div class="mediaCard ${on?'on':''}" data-picker-key="${esc(key)}" title="${esc(row?.url || row?.remote?.path || '')}"><div class="thumb">${previewHTML(row)}<span class="type">${esc(typeLabel(row))}</span><span class="pick">${on?'✓':''}</span></div><div class="meta"><div class="name">${esc(name)}</div><div class="sub">${esc(sub)}</div>${qualitiesHTML(row)}${sourceLink}</div></div>`;
    }).join('');
    grid.scrollTop = 0;
    grid.querySelectorAll('.mediaCard').forEach(card => card.addEventListener('click', event => {
      if (event.target?.closest?.('video,audio,button,input,a')) return;
      const key = String(card.dataset.pickerKey || '');
      if (!key) return;
      selected.has(key) ? selected.delete(key) : selected.add(key);
      render();
    }));
  }

  function syncChrome() {
    const discovery = pickerMode === 'discovery';
    const stage = document.getElementById('ddgMediaPickerStageV8566');
    if (stage) {
      const warning = discovery && discoveryWarnings.length ? ` <span>• ${esc(discoveryWarnings.join(' • '))}</span>` : '';
      stage.innerHTML = discovery ? `<b>Pasul 1/2:</b> verifică preview-ul și calitățile, apoi alege ce compari.${warning}` : '<b>Pasul 2/2:</b> rezultatele au fost comparate cu PC-ul; în JD pleacă numai lipsurile selectate.';
    }
    const primary = document.getElementById('ddgMediaPickerSendSelectedV8566');
    if (primary) primary.textContent = discovery ? 'Compară selectatele cu PC' : 'Trimite lipsurile selectate în JD';
    const recommended = document.getElementById('ddgMediaPickerRecommendedV8566');
    if (recommended) recommended.textContent = discovery ? 'Selectează video' : 'Selectează lipsurile';
    const refreshButton = document.getElementById('ddgMediaPickerRefreshV8566');
    if (refreshButton) refreshButton.textContent = discovery ? '↻ Reanalizează pagina' : '↻ Reîncarcă rezultate';
  }

  function showLoading(title, detail = '') {
    releaseGridMedia();
    const grid = document.getElementById('ddgMediaPickerGridV8566');
    if (grid) grid.innerHTML = `<div class="previewLoading"><div class="spin"></div><b>${esc(title)}</b><span class="small">${esc(detail)}</span></div>`;
  }

  function showError(error) {
    const grid = document.getElementById('ddgMediaPickerGridV8566');
    if (grid) grid.innerHTML = `<div class="previewEmpty">${esc(error?.message || String(error))}</div>`;
  }

  function candidateActionURLs(candidates) {
    const seen = new Set();
    const urls = [];
    for (const row of candidates || []) {
      const raw = String(row?.url || '').trim();
      if (!/^https?:\/\//i.test(raw) || seen.has(raw)) continue;
      seen.add(raw); urls.push(raw);
    }
    return urls;
  }

  async function compareSelected() {
    const candidates = rows.filter(row => selected.has(rowKey(row)));
    const urls = candidateActionURLs(candidates);
    if (!urls.length) return window.toast?.('Selectează cel puțin o sursă media.');
    loading = true;
    showLoading(`Compar ${urls.length} selecții cu PC-ul…`, 'Rulez extractoarele numai pentru elementele alese.');
    try {
      const mode = document.getElementById('mode')?.value || 'balanced';
      const data = await window.api('/api/source/batch', {method:'POST', headers:{'Content-Type':'application/json'}, body:JSON.stringify({urls, mode, adapter:'auto'})});
      pickerMode = 'results'; rows = []; selected.clear(); pageIndex = 0; loading = false;
      await loadRows(true);
      for (const row of rows) if (isRecommended(row)) selected.add(rowKey(row));
      render();
      window.dispatchEvent(new CustomEvent('ddg:source-scan-complete', {detail:{url:sourceURL, provider:'web', adapter:'generic-selected', items:Number(data?.items || 0), selectedSources:urls.length, mediaPicker:true}}));
      window.toast?.(`${Number(data?.items || 0).toLocaleString('ro-RO')} fișier(e) comparate; lipsurile sunt selectate.`);
    } catch (error) {
      loading = false; showError(error);
      window.dispatchEvent(new CustomEvent('ddg:source-scan-error', {detail:{url:sourceURL, provider:'web', message:String(error?.message || error), mediaPicker:true}}));
    }
  }

  async function sendIDs(ids) {
    const unique = [...new Set((ids || []).map(Number).filter(Number.isFinite))];
    if (!unique.length) return window.toast?.('Nu există fișiere de trimis');
    const jd = window.ddgJDownloaderFastV8566;
    if (!jd?.sendExactIDs) return window.toast?.('Modulul JDownloader rapid nu este încă încărcat.');
    try { await jd.sendExactIDs(unique, {confirm:true}); } catch (_) {}
  }

  function primaryAction() {
    if (pickerMode === 'discovery') return void compareSelected();
    const ids = rows.filter(row => selected.has(rowKey(row))).map(row => Number(row.id));
    void sendIDs(ids);
  }

  function resetView() {
    releaseGridMedia(); rows = []; selected.clear(); activeFilter = 'all'; query = ''; pageIndex = 0; discoveryWarnings = [];
    const search = document.getElementById('ddgMediaPickerSearchV8566'); if (search) search.value = '';
    document.querySelectorAll('#ddgMediaPickerV8566 [data-picker-filter]').forEach(x => x.classList.toggle('on', x.dataset.pickerFilter === 'all'));
  }

  async function open(raw = '') {
    installUI();
    sourceURL = String(raw || document.getElementById('directUrl')?.value || '').trim();
    if (!genericPickerAllowed(sourceURL)) return window.toast?.('Media Picker este disponibil numai pentru site-uri generice; acest link folosește providerul dedicat.');
    pickerMode = 'results'; resetView();
    document.getElementById('ddgMediaPickerTitleV8566').textContent = `Media Picker — ${providerName(sourceURL)}`;
    document.getElementById('ddgMediaPickerV8566')?.classList.remove('hidden');
    await loadRows(true);
  }

  async function openDiscovery(raw = '', discovery = {}) {
    installUI();
    sourceURL = String(raw || discovery?.url || '').trim();
    if (!genericPickerAllowed(sourceURL)) return;
    pickerMode = 'discovery'; resetView();
    discoveryWarnings = Array.isArray(discovery?.warnings) ? discovery.warnings.filter(Boolean) : [];
    const candidates = Array.isArray(discovery?.candidates) ? discovery.candidates : [];
    rows = candidates.map((row, index) => ({...row, _pickerKey:`discovery:${String(row?.id || row?.token || index)}:${index}`}));
    if (!rows.length) rows = [{_pickerKey:'discovery:fallback:0', id:'fallback', url:sourceURL, title:'Pagina sursă — analiză automată la comparare', kind:'page', via:'fallback auto'}];
    if (rows.some(row => mediaKind(row) === 'video')) {
      activeFilter = 'video';
      document.querySelectorAll('#ddgMediaPickerV8566 [data-picker-filter]').forEach(x => x.classList.toggle('on', x.dataset.pickerFilter === 'video'));
    }
    for (const row of rows) if (isRecommended(row)) selected.add(rowKey(row));
    document.getElementById('ddgMediaPickerTitleV8566').textContent = `Media Picker — ${providerName(sourceURL)}`;
    document.getElementById('ddgMediaPickerV8566')?.classList.remove('hidden');
    render();
  }

  async function refresh() {
    if (pickerMode === 'discovery' && window.ddgGenericMediaDiscoveryV85127?.run) {
      close();
      return window.ddgGenericMediaDiscoveryV85127.run(true);
    }
    return loadRows(true);
  }

  function close() {
    releaseGridMedia();
    const grid = document.getElementById('ddgMediaPickerGridV8566'); if (grid) grid.innerHTML = '';
    document.getElementById('ddgMediaPickerV8566')?.classList.add('hidden');
  }

  function boot() { installUI(); setTimeout(installLaunchButton, 500); setTimeout(installLaunchButton, 1500); }
  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', boot, {once:true}); else boot();
  window.ddgMediaPickerV8566 = {open, openDiscovery, close, reload:() => loadRows(true), genericPickerAllowed, candidateActionURLs};
})();
