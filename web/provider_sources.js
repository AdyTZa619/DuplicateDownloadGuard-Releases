(() => {
  'use strict';

  const LAST_SOURCE_KEY = 'ddg.lastUniversalSourceUrl';
  const PROVIDERS = [
    {id:'mega', name:'MEGA', adapter:'mega', hosts:[/^mega\.nz$/i, /^mega\.co\.nz$/i], note:'motor MEGAcmd dedicat'},
    {id:'gofile', name:'GoFile', adapter:'gallery-dl', hosts:[/(^|\.)gofile\.io$/i], note:'gallery-dl • foldere recursive'},
    {id:'bunkr', name:'Bunkr', adapter:'gallery-dl', hosts:[/(^|\.)bunkr[a-z0-9-]*\.[a-z]{2,}$/i, /(^|\.)bunkrr\.[a-z]{2,}$/i], note:'gallery-dl • albume/media'},
    {id:'cyberdrop', name:'Cyberdrop', adapter:'gallery-dl', hosts:[/(^|\.)cyberdrop\.[a-z]{2,}$/i], note:'gallery-dl • albume/media'},
    {id:'erome', name:'Erome', adapter:'gallery-dl', hosts:[/(^|\.)erome\.com$/i], note:'gallery-dl • albume/media'},
    {id:'pixeldrain', name:'Pixeldrain', adapter:'auto', hosts:[/(^|\.)pixeldrain\.com$/i], note:'detecție automată'},
    {id:'mediafire', name:'MediaFire', adapter:'auto', hosts:[/(^|\.)mediafire\.com$/i], note:'detecție automată'}
  ];

  let galleryReady = null;
  let galleryCheckAt = 0;
  let scanInFlight = false;
  let organizeAttempts = 0;

  function rememberLastSource(value) {
    const raw = String(value || '').trim();
    if (!raw) return;
    try { localStorage.setItem(LAST_SOURCE_KEY, raw); } catch (_) {}
  }

  function restoreLastSource(input) {
    if (!input || String(input.value || '').trim()) return;
    try {
      const saved = String(localStorage.getItem(LAST_SOURCE_KEY) || '').trim();
      if (saved) input.value = saved;
    } catch (_) {}
  }

  function parseURL(raw) {
    try {
      const value = String(raw || '').trim();
      if (!value) return null;
      return new URL(/^https?:\/\//i.test(value) ? value : 'https://' + value);
    } catch (_) {
      return null;
    }
  }

  function detectProvider(raw) {
    const u = parseURL(raw);
    if (!u) return null;
    const host = u.hostname.toLowerCase();
    for (const p of PROVIDERS) {
      if (p.hosts.some(rx => rx.test(host))) return {...p, host};
    }
    if (u.protocol === 'http:' || u.protocol === 'https:') {
      return {id:'web', name:'Web / HTTP', adapter:'auto', host, note:'HTTP → yt-dlp → gallery-dl'};
    }
    return null;
  }

  async function jsonFetch(url, options) {
    const response = await fetch(url, options);
    if (!response.ok) throw new Error((await response.text()).trim() || `HTTP ${response.status}`);
    const type = response.headers.get('content-type') || '';
    return type.includes('json') ? response.json() : response.text();
  }

  async function checkGalleryDL(force = false) {
    if (!force && galleryReady !== null && Date.now() - galleryCheckAt < 15000) return galleryReady;
    galleryCheckAt = Date.now();
    try {
      const tools = await jsonFetch('/api/tools');
      const item = Array.isArray(tools) ? tools.find(x => String(x.name || '').toLowerCase() === 'gallery-dl') : null;
      galleryReady = !!item?.found;
    } catch (_) {
      galleryReady = null;
    }
    renderProviderState();
    return galleryReady;
  }

  function stateBox() {
    return document.getElementById('universalProviderState');
  }

  function scanButton() {
    return document.getElementById('universalScanButton');
  }

  function setScanBusy(busy, provider) {
    const button = scanButton();
    if (!button) return;
    button.disabled = !!busy;
    button.textContent = busy
      ? `Analizez ${provider?.name || 'sursa'}…`
      : 'Analizează fără download';
  }

  function escapeHTML(value) {
    return String(value ?? '').replace(/[&<>"']/g, ch => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[ch]));
  }

  function ensureSourceIntelligence(body, input) {
    let card = document.getElementById('ddgSourceIntelligenceV85124');
    if (card) return card;
    if (!body || !input) return null;

    card = document.createElement('div');
    card.id = 'ddgSourceIntelligenceV85124';
    card.className = 'ddgSourceIntelligenceV85124';
    card.innerHTML = `
      <div class="sourceIntelHeadV85124">
        <div><b>Asistent sursă</b><span>provider • istoric • folder • media</span></div>
        <span class="sourcePill">fără rescanare HDD</span>
      </div>
      <div id="universalProviderState" class="sourceIntelProviderV85124"><span class="muted">Lipește un link. DDG va alege automat motorul potrivit.</span></div>
      <div class="sourceIntelGridV85124">
        <section class="sourceIntelBlockV85124">
          <div class="sourceIntelLabelV85124">ISTORIC LINK</div>
          <div id="ddgSourceHistorySlotV85124" class="sourceIntelSlotV85124"><span class="muted small">Apare imediat dacă sursa a mai fost verificată.</span></div>
        </section>
        <section class="sourceIntelBlockV85124">
          <div class="sourceIntelLabelV85124">FOLDER RECOMANDAT</div>
          <div id="ddgSourceFolderSlotV85124" class="sourceIntelSlotV85124"><span class="muted small">După scanare, DDG indică folderul local cel mai legat de sursă.</span></div>
        </section>
        <section class="sourceIntelBlockV85124">
          <div class="sourceIntelLabelV85124">MEDIA</div>
          <div id="ddgSourceMediaSlotV85124" class="sourceIntelSlotV85124">
            <div id="ddgSourceMediaInfoV85124" class="muted small">Media Picker folosește rezultatele scanării curente.</div>
            <div id="ddgSourceMediaActionsV85124" class="sourceIntelActionsV85124"></div>
          </div>
        </section>
      </div>
      <div class="providerHostList"><span>GoFile</span><span>Bunkr</span><span>Cyberdrop</span><span>Erome</span><span>MEGA</span><span>HTTP</span></div>`;

    const firstRow = input.nextElementSibling;
    if (firstRow) firstRow.insertAdjacentElement('afterend', card);
    else body.appendChild(card);
    return card;
  }

  function organizeSourceTools() {
    const actions = document.getElementById('ddgSourceMediaActionsV85124');
    const pickerButton = document.getElementById('ddgMediaPickerLaunchV8566');
    if (actions && pickerButton && pickerButton.parentElement !== actions) {
      actions.appendChild(pickerButton);
      pickerButton.style.marginTop = '0';
    }
    if ((!actions || !pickerButton) && organizeAttempts < 24) {
      organizeAttempts++;
      setTimeout(organizeSourceTools, 250);
    }
  }

  function renderProviderState() {
    const input = document.getElementById('directUrl');
    const box = stateBox();
    if (!input || !box) return;
    const p = detectProvider(input.value);
    const card = document.getElementById('ddgSourceIntelligenceV85124');
    if (card) card.dataset.provider = p?.id || '';
    if (!p) {
      box.innerHTML = '<span class="muted">Lipește un link. DDG va alege automat motorul potrivit.</span>';
      return;
    }
    const needsGallery = p.adapter === 'gallery-dl';
    const ready = !needsGallery
      ? ''
      : galleryReady === true
        ? '<span class="badge VERIFIED">MOTOR GATA</span>'
        : galleryReady === false
          ? '<span class="badge DIFFERENT">GALLERY-DL LIPSEȘTE</span>'
          : '<span class="badge POSSIBLE">VERIFIC MOTORUL</span>';
    const action = needsGallery && galleryReady === false
      ? '<button class="btn" id="prepareGalleryDL" type="button">Instalează / actualizează gallery-dl</button>'
      : '';
    box.innerHTML = `<div class="providerStateLine"><b>${escapeHTML(p.name)}</b><span class="sourcePill">${escapeHTML(p.note)}</span>${ready}</div><div class="muted small providerStateHint">DDG listează și compară metadata/URL-urile extrase; nu descarcă întâi tot conținutul.</div>${action}`;
    document.getElementById('prepareGalleryDL')?.addEventListener('click', installGalleryDL);
  }

  async function installGalleryDL() {
    const button = document.getElementById('prepareGalleryDL');
    if (button) {
      button.disabled = true;
      button.textContent = 'Pregătesc gallery-dl…';
    }
    try {
      await jsonFetch('/api/tools/manage', {
        method:'POST',
        headers:{'Content-Type':'application/json'},
        body:JSON.stringify({tool:'gallery-dl', action:'install'})
      });
      window.toast?.('Instalarea/actualizarea gallery-dl a pornit');
      galleryReady = null;
      galleryCheckAt = 0;
      setTimeout(() => checkGalleryDL(true), 2500);
    } catch (err) {
      window.toast?.(err.message);
      if (button) {
        button.disabled = false;
        button.textContent = 'Reîncearcă gallery-dl';
      }
    }
  }

  async function universalScanV2() {
    if (scanInFlight) return;

    const input = document.getElementById('directUrl');
    const select = document.getElementById('sourceAdapter');
    const raw = String(input?.value || '').trim();
    if (!raw) {
      window.toast?.('Lipește un link');
      return;
    }

    rememberLastSource(raw);
    const provider = detectProvider(raw);
    if (provider?.id === 'mega') {
      const mega = document.getElementById('megaUrl');
      if (mega) mega.value = raw;
      if (typeof window.scanMega === 'function') return window.scanMega();
    }

    scanInFlight = true;
    setScanBusy(true, provider);
    const top = document.getElementById('topStatus');
    if (top) top.textContent = provider ? `Analizez ${provider.name}…` : 'Analizez sursa…';
    window.dispatchEvent(new CustomEvent('ddg:source-scan-start', {detail:{url:raw, provider:provider?.id || 'web'}}));

    try {
      let adapter = String(select?.value || 'auto').toLowerCase();
      if (adapter === 'auto' && provider?.adapter && provider.adapter !== 'auto') adapter = provider.adapter;
      if (adapter === 'gallery-dl') {
        const ready = await checkGalleryDL();
        if (ready === false) {
          renderProviderState();
          window.toast?.('gallery-dl lipsește. Folosește butonul de instalare/actualizare din Asistent sursă.');
          return;
        }
      }

      const mode = document.getElementById('mode')?.value || 'balanced';
      const genericWeb = provider?.id === 'web';
      const endpoint = genericWeb ? '/api/source/batch' : '/api/source/scan';
      const payload = genericWeb ? {urls:[raw], mode, adapter} : {url:raw, mode, adapter};
      const data = await jsonFetch(endpoint, {
        method:'POST',
        headers:{'Content-Type':'application/json'},
        body:JSON.stringify(payload)
      });
      const used = provider?.name || data.adapter || adapter;
      const items = Number(data.items || 0);
      window.toast?.(`${used}: ${items.toLocaleString('ro-RO')} fișier(e) comparate`);
      if (top) top.textContent = `${used}: analiză terminată`;
      window.dispatchEvent(new CustomEvent('ddg:source-scan-complete', {detail:{url:raw, provider:provider?.id || 'web', adapter, items}}));
      if (typeof window.goTab === 'function') window.goTab('results');
    } catch (err) {
      const message = String(err?.message || err || 'Eroare necunoscută');
      window.toast?.(message);
      if (top) top.textContent = `Eroare sursă: ${message}`;
      window.dispatchEvent(new CustomEvent('ddg:source-scan-error', {detail:{url:raw, provider:provider?.id || 'web', message}}));
    } finally {
      scanInFlight = false;
      setScanBusy(false, provider);
    }
  }

  function bindUniversalScanButton(body) {
    let button = document.getElementById('universalScanButton');
    if (!button) button = body?.querySelector('button[onclick="scanUniversal()"]');
    if (!button) return;

    button.id = 'universalScanButton';
    button.removeAttribute('onclick');
    if (button.dataset.ddgProviderBound === '1') return;
    button.dataset.ddgProviderBound = '1';
    button.addEventListener('click', universalScanV2);
  }

  function loadFeatureModule(id, src) {
    if (document.getElementById(id)) return;
    const script = document.createElement('script');
    script.id = id;
    script.defer = true;
    script.src = src;
    document.head.appendChild(script);
  }

  function loadFeatureModules() {
    loadFeatureModule('ddgGenericMediaPickerScriptV85114', '/feature_generic_media_picker_v85114.js');
    loadFeatureModule('ddgSourceFolderHintScriptV85114', '/feature_source_folder_hint_v85114.js');
    loadFeatureModule('ddgSourceHistoryScriptV85124', '/features/source_history_v85124.js');
  }

  function ensureStyles() {
    if (document.getElementById('providerSourceStyles')) return;
    const style = document.createElement('style');
    style.id = 'providerSourceStyles';
    style.textContent = `
      .ddgSourceIntelligenceV85124{margin-top:10px;border:1px solid #2c3c4e;background:#0b121a;border-radius:11px;overflow:hidden}
      .sourceIntelHeadV85124{display:flex;gap:10px;align-items:center;justify-content:space-between;padding:10px 12px;border-bottom:1px solid #223141;background:#0d151e}
      .sourceIntelHeadV85124>div{display:flex;gap:8px;align-items:center;flex-wrap:wrap}.sourceIntelHeadV85124>div>span{font-size:10px;color:#8298ae}
      .sourceIntelProviderV85124{padding:10px 12px;border-bottom:1px solid #1d2a37}.providerStateLine{display:flex;gap:8px;align-items:center;flex-wrap:wrap}.providerStateHint{margin-top:5px}.sourceIntelProviderV85124 .btn{margin-top:8px}
      .sourceIntelGridV85124{display:grid;grid-template-columns:1.1fr 1fr 1fr;gap:1px;background:#223141}
      .sourceIntelBlockV85124{background:#0b131c;padding:10px 11px;min-width:0}.sourceIntelLabelV85124{font-size:9px;font-weight:850;letter-spacing:.08em;color:#6f8ba6;margin-bottom:7px}
      .sourceIntelSlotV85124{min-height:48px;min-width:0}.sourceIntelActionsV85124{display:flex;gap:7px;align-items:center;flex-wrap:wrap;margin-top:8px}
      .providerHostList{display:flex;gap:6px;flex-wrap:wrap;padding:8px 11px;border-top:1px solid #1d2a37;background:#0a1118}
      .providerHostList span{font-size:9px;font-weight:750;color:#8fa6bc;border:1px solid #293b4d;border-radius:999px;padding:2px 6px;background:#0e1720}
      .ddgSourceIntelligenceV85124 .ddgFolderHintV85114{margin:0;padding:0;border:0;background:transparent;border-radius:0}
      .ddgSourceIntelligenceV85124 #ddgMediaPickerLaunchV8566{margin-top:0}
      @media(max-width:980px){.sourceIntelGridV85124{grid-template-columns:1fr}.sourceIntelBlockV85124{border-bottom:1px solid #223141}.sourceIntelBlockV85124:last-child{border-bottom:0}}
    `;
    document.head.appendChild(style);
  }

  function installUI() {
    const input = document.getElementById('directUrl');
    if (!input) return;
    restoreLastSource(input);
    input.placeholder = 'GoFile, Bunkr, Cyberdrop, Erome, MEGA, link direct, galerie sau pagină video…';
    const section = input.closest('.section');
    const head = section?.querySelector('.sectionHead h3');
    if (head) head.textContent = 'Sursă online — universal';
    const body = input.closest('.sectionBody');

    ensureStyles();
    ensureSourceIntelligence(body, input);
    bindUniversalScanButton(body);

    if (input.dataset.ddgProviderInputBound !== '1') {
      input.dataset.ddgProviderInputBound = '1';
      input.addEventListener('input', renderProviderState);
      input.addEventListener('paste', () => setTimeout(renderProviderState, 0));
      input.addEventListener('keydown', event => {
        if (event.key === 'Enter') {
          event.preventDefault();
          universalScanV2();
        }
      });
    }

    window.scanUniversal = universalScanV2;
    renderProviderState();
    checkGalleryDL();
    loadFeatureModules();
    organizeSourceTools();
  }

  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', installUI, {once:true});
  else installUI();
})();
