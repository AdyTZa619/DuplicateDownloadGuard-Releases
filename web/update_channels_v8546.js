// DDG 8.5.48 — Stable + TEST updater channels.
// TEST reads manifest + EXE from one pinned Git commit via GitHub REST JSON/base64,
// so browser CORS and moving-branch SHA races cannot corrupt the update.
(() => {
  'use strict';

  const REPO_API = 'https://api.github.com/repos/AdyTZa619/DuplicateDownloadGuard-Releases';
  const TEST_BRANCH_REF_API = `${REPO_API}/git/ref/heads/testing`;
  const CORNER_ID = 'ddgUpdateCorner';
  const TEST_BOX_ID = 'ddgTestUpdaterBox';
  let stableState = null;
  let testState = null;
  let currentVersion = '';
  let checking = false;
  let cornerObserver = null;

  const sleep = ms => new Promise(resolve => setTimeout(resolve, ms));

  function addStyles() {
    if (document.getElementById('ddgUpdateChannelsStyle')) return;
    const style = document.createElement('style');
    style.id = 'ddgUpdateChannelsStyle';
    style.textContent = `
      #${CORNER_ID}{display:inline-flex;align-items:center;gap:7px;margin-left:auto;margin-right:8px;vertical-align:middle;flex-shrink:0}
      #${CORNER_ID} .ddgUpdateChip{border:1px solid #3c5066;background:#172131;color:#eaf2ff;padding:7px 10px;border-radius:8px;cursor:pointer;font-weight:750;line-height:1}
      #${CORNER_ID} .ddgUpdateChip:hover{border-color:#4da3ff}
      #${CORNER_ID} .ddgUpdateChip.stable{border-color:#2d6f58;background:#10281f;color:#8ff0c8}
      #${CORNER_ID} .ddgUpdateChip.test{border-color:#6d57a2;background:#211a32;color:#d2c0ff}
      #${CORNER_ID} .ddgUpdateChip.busy{opacity:.65;cursor:wait}
      .ddgTestUpdater{margin-top:14px;border:1px solid #4d3f72;background:#171326;border-radius:10px;padding:13px}
      .ddgTestUpdaterHead{display:flex;align-items:center;gap:8px;margin-bottom:7px}
      .ddgTestBadge{display:inline-flex;padding:3px 6px;border:1px solid #6d57a2;border-radius:999px;background:#2d244b;color:#d8caff;font-size:10px;font-weight:800;letter-spacing:.08em}
      .ddgTestActions{display:flex;gap:8px;flex-wrap:wrap;margin-top:10px}
    `;
    document.head.appendChild(style);
  }

  function placeCorner(box) {
    const top = document.querySelector('.top');
    if (!top || !box) return box || null;
    const hud = document.getElementById('operationHud');
    const status = document.getElementById('topStatus');
    const statusHost = status?.parentElement;
    if (hud && hud.parentElement === top) {
      if (box.parentElement !== top || box.nextElementSibling !== hud) top.insertBefore(box, hud);
    } else if (statusHost && statusHost.parentElement === top) {
      if (box.parentElement !== top || box.nextElementSibling !== statusHost) top.insertBefore(box, statusHost);
    } else if (box.parentElement !== top) {
      top.appendChild(box);
    }
    return box;
  }

  function ensureCorner() {
    addStyles();
    let box = document.getElementById(CORNER_ID);
    if (!box) {
      box = document.createElement('span');
      box.id = CORNER_ID;
    }
    return placeCorner(box);
  }

  function keepCornerMounted() {
    let tries = 0;
    const timer = setInterval(() => {
      tries++;
      placeCorner(ensureCorner());
      if (tries >= 80) clearInterval(timer);
    }, 100);
    if (!cornerObserver && document.body) {
      cornerObserver = new MutationObserver(() => {
        const box = document.getElementById(CORNER_ID);
        if (box) placeCorner(box);
      });
      cornerObserver.observe(document.body, {childList: true, subtree: true});
    }
  }

  function ensureTestBox() {
    if (document.getElementById(TEST_BOX_ID)) return;
    const stableText = document.getElementById('updateStatusText');
    if (!stableText || !stableText.parentElement) return;
    const box = document.createElement('div');
    box.id = TEST_BOX_ID;
    box.className = 'ddgTestUpdater';
    box.innerHTML = `
      <div class="ddgTestUpdaterHead"><span class="ddgTestBadge">TEST</span><b>Canal separat pentru versiuni de probă</b></div>
      <div class="muted small">Build-urile TEST sunt separate de Stable. Manifestul și EXE-ul sunt citite din aceeași revizie Git și verificate SHA-256 înainte de instalare.</div>
      <div class="ddgTestActions"><button class="btn" id="ddgCheckTestUpdate">↻ Verifică TEST</button><button class="btn" id="ddgInstallTestUpdate">⬇ Instalează TEST</button></div>
      <div class="muted small" id="ddgTestUpdateStatus" style="margin-top:8px">Nu a fost verificat încă.</div>`;
    stableText.parentElement.appendChild(box);
    document.getElementById('ddgCheckTestUpdate').onclick = () => checkTest(false);
    document.getElementById('ddgInstallTestUpdate').onclick = () => installTest();
  }

  function parseVersion(value) {
    const m = String(value || '').match(/(\d+)\.(\d+)\.(\d+)(?:-([0-9a-z.-]+))?/i);
    if (!m) return null;
    return {core: [+m[1], +m[2], +m[3]], pre: m[4] ? m[4].toLowerCase().split('.') : []};
  }

  function comparePre(a, b) {
    if (!a.length && !b.length) return 0;
    if (!a.length) return 1;
    if (!b.length) return -1;
    for (let i = 0; i < Math.min(a.length, b.length); i++) {
      if (a[i] === b[i]) continue;
      const an = /^\d+$/.test(a[i]);
      const bn = /^\d+$/.test(b[i]);
      if (an && bn) return Number(a[i]) > Number(b[i]) ? 1 : -1;
      if (an !== bn) return an ? -1 : 1;
      return a[i] > b[i] ? 1 : -1;
    }
    return a.length === b.length ? 0 : (a.length > b.length ? 1 : -1);
  }

  function isNewer(remote, local) {
    const a = parseVersion(remote);
    const b = parseVersion(local);
    if (!a || !b) return false;
    for (let i = 0; i < 3; i++) {
      if (a.core[i] !== b.core[i]) return a.core[i] > b.core[i];
    }
    return comparePre(a.pre, b.pre) > 0;
  }

  async function localVersion() {
    if (currentVersion) return currentVersion;
    const info = await window.api('/api/about');
    currentVersion = info.version || '';
    return currentVersion;
  }

  async function githubJSON(url) {
    const response = await fetch(url, {
      cache: 'no-store',
      headers: {
        Accept: 'application/vnd.github+json',
        'X-GitHub-Api-Version': '2022-11-28'
      }
    });
    if (!response.ok) {
      let detail = '';
      try { detail = (await response.text()).trim(); } catch (_) {}
      throw new Error(`GitHub API HTTP ${response.status}${detail ? ` — ${detail.slice(0, 160)}` : ''}`);
    }
    return await response.json();
  }

  function decodeBase64(value) {
    const clean = String(value || '').replace(/\s+/g, '');
    if (!clean) throw new Error('GitHub nu a returnat conținutul fișierului');
    const raw = atob(clean);
    const out = new Uint8Array(raw.length);
    for (let i = 0; i < raw.length; i++) out[i] = raw.charCodeAt(i);
    return out;
  }

  async function contentsBytes(path, ref) {
    const meta = await githubJSON(`${REPO_API}/contents/${path}?ref=${encodeURIComponent(ref)}`);
    if (meta?.encoding === 'base64' && meta?.content) return decodeBase64(meta.content);
    const blobURL = meta?.git_url || (meta?.sha ? `${REPO_API}/git/blobs/${meta.sha}` : '');
    if (!blobURL) throw new Error(`GitHub nu a furnizat blob-ul pentru ${path}`);
    const blob = await githubJSON(blobURL);
    if (blob?.encoding !== 'base64' || !blob?.content) {
      throw new Error(`GitHub nu a furnizat conținut base64 pentru ${path}`);
    }
    return decodeBase64(blob.content);
  }

  async function fetchTestSnapshot() {
    const branch = await githubJSON(TEST_BRANCH_REF_API);
    const ref = String(branch?.object?.sha || '').trim();
    if (!/^[0-9a-f]{40}$/i.test(ref)) throw new Error('Nu pot fixa revizia canalului TEST');
    const manifestBytes = await contentsBytes('update-test.json', ref);
    let manifest;
    try {
      manifest = JSON.parse(new TextDecoder('utf-8').decode(manifestBytes).replace(/^\uFEFF/, ''));
    } catch (_) {
      throw new Error('manifest TEST invalid');
    }
    if (!manifest?.version || !manifest?.sha256) throw new Error('manifest TEST incomplet');
    return {ref, manifest};
  }

  async function checkStable(silent = true) {
    try {
      stableState = await window.api('/api/update/check');
      renderCorner();
      if (stableState?.newer && !silent && typeof window.toast === 'function') {
        window.toast(`Update Stable disponibil: ${stableState.manifest.version}`);
      }
      return stableState;
    } catch (_) {
      stableState = null;
      renderCorner();
      return null;
    }
  }

  async function checkTest(silent = true) {
    const text = document.getElementById('ddgTestUpdateStatus');
    try {
      const [{ref, manifest}, installed] = await Promise.all([fetchTestSnapshot(), localVersion()]);
      testState = {configured: true, ref, manifest, newer: isNewer(manifest.version, installed)};
      if (text) {
        text.textContent = testState.newer
          ? `TEST disponibil: ${manifest.version} — ${manifest.notes || ''}`
          : `Niciun TEST mai nou • ${manifest.version}`;
      }
      renderCorner();
      if (testState.newer && !silent && typeof window.toast === 'function') {
        window.toast(`Versiune TEST disponibilă: ${manifest.version}`);
      }
      return testState;
    } catch (err) {
      testState = null;
      if (text) text.textContent = `Canal TEST indisponibil momentan: ${err.message}`;
      renderCorner();
      return null;
    }
  }

  function renderCorner() {
    const box = ensureCorner();
    if (!box) return;
    const html = [];
    if (stableState?.newer) {
      html.push(`<button class="ddgUpdateChip stable" data-channel="stable" title="Instalează update-ul Stable">↑ Update ${stableState.manifest.version}</button>`);
    }
    if (testState?.newer) {
      html.push(`<button class="ddgUpdateChip test" data-channel="test" title="Instalează versiunea TEST">TEST ${testState.manifest.version}</button>`);
    }
    box.innerHTML = html.join('');
    box.querySelectorAll('[data-channel="stable"]').forEach(btn => btn.onclick = () => installStable(btn));
    box.querySelectorAll('[data-channel="test"]').forEach(btn => btn.onclick = () => installTest(btn));
    placeCorner(box);
  }

  async function installStable(button) {
    const version = stableState?.manifest?.version || 'Stable';
    if (!confirm(`Instalez DDG ${version} Stable? Aplicația se va închide și reporni automat.`)) return;
    if (button) {
      button.disabled = true;
      button.classList.add('busy');
      button.textContent = 'Se instalează…';
    }
    try {
      await window.api('/api/update/install-online', {method: 'POST'});
    } catch (err) {
      if (typeof window.toast === 'function') window.toast('Update Stable: ' + err.message);
      if (button) {
        button.disabled = false;
        button.classList.remove('busy');
        button.textContent = `↑ Update ${version}`;
      }
    }
  }

  async function sha256Hex(bytes) {
    const view = bytes instanceof Uint8Array ? bytes : new Uint8Array(bytes);
    const digest = await crypto.subtle.digest('SHA-256', view);
    return [...new Uint8Array(digest)].map(b => b.toString(16).padStart(2, '0')).join('');
  }

  async function installTest(button) {
    try {
      if (!testState?.newer || !testState?.ref) await checkTest(false);
      if (!testState?.newer || !testState?.ref) {
        if (typeof window.toast === 'function') window.toast('Nu există un build TEST mai nou.');
        return;
      }
      const {manifest, ref} = testState;
      if (!confirm(`Instalez DDG ${manifest.version} din canalul TEST?\n\nEste un build de probă. Updaterul păstrează backup și face rollback automat dacă noua versiune nu pornește.`)) return;
      const btn = button || document.getElementById('ddgInstallTestUpdate');
      if (btn) {
        btn.disabled = true;
        btn.classList.add('busy');
        btn.textContent = 'Descarc TEST…';
      }
      const bytes = await contentsBytes('test-releases/DuplicateDownloadGuard_PRO_TEST.exe', ref);
      if (bytes.byteLength > 100 * 1024 * 1024) throw new Error('build TEST prea mare (>100 MB)');
      const got = await sha256Hex(bytes);
      if (got.toLowerCase() !== String(manifest.sha256).trim().toLowerCase()) {
        throw new Error(`SHA-256 TEST diferit chiar în aceeași revizie Git (${got.slice(0,12)}… != ${String(manifest.sha256).slice(0,12)}…)`);
      }
      if (btn) btn.textContent = 'Aplic TEST…';
      const form = new FormData();
      form.append('file', new Blob([bytes], {type: 'application/vnd.microsoft.portable-executable'}), `DuplicateDownloadGuard_TEST_${manifest.version}.exe`);
      const apply = await fetch('/api/update/apply', {method: 'POST', body: form});
      if (!apply.ok) throw new Error((await apply.text()).trim() || `updater local HTTP ${apply.status}`);
      await apply.json();
    } catch (err) {
      if (typeof window.toast === 'function') window.toast('Update TEST: ' + err.message);
      const btn = button || document.getElementById('ddgInstallTestUpdate');
      if (btn) {
        btn.disabled = false;
        btn.classList.remove('busy');
        btn.textContent = '⬇ Instalează TEST';
      }
    }
  }

  async function checkAll() {
    if (checking || typeof window.api !== 'function') return;
    checking = true;
    try {
      await Promise.all([checkStable(true), checkTest(true)]);
    } finally {
      checking = false;
    }
  }

  async function boot() {
    for (let i = 0; i < 40 && typeof window.api !== 'function'; i++) await sleep(100);
    if (typeof window.api !== 'function') return;
    ensureCorner();
    keepCornerMounted();
    ensureTestBox();
    setTimeout(checkAll, 1400);
    setInterval(checkAll, 10 * 60 * 1000);
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', boot, {once: true});
  } else {
    boot();
  }

  window.ddgCheckAllUpdateChannels = checkAll;
})();
