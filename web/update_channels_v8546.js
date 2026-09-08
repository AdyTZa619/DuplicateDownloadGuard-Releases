// DDG Stable + TEST updater channels.
// TEST discovery/install deliberately avoids api.github.com so public GitHub API
// rate limits cannot block the updater. The moving raw branch is protected by
// SHA-256 verification and a bounded re-fetch on publish/cache races.
(() => {
  'use strict';

  const RAW_ROOT = 'https://raw.githubusercontent.com/AdyTZa619/DuplicateDownloadGuard-Releases';
  const TEST_MANIFEST_URL = `${RAW_ROOT}/testing/update-test.json`;
  const DEFAULT_TEST_EXE_URL = `${RAW_ROOT}/testing/test-releases/DuplicateDownloadGuard_PRO_TEST.exe`;
  const RECOVERY_PORTS = [51289, 51290, 51291, 51292];
  const CORNER_ID = 'ddgUpdateCorner';
  const TEST_BOX_ID = 'ddgTestUpdaterBox';
  let stableState = null;
  let testState = null;
  let currentVersion = '';
  let currentAppDir = '';
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
      <div class="muted small">Canalul TEST folosește fișiere raw GitHub, fără API public și fără limita de 60 cereri/oră. EXE-ul este verificat SHA-256 înainte de instalare.</div>
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
    currentAppDir = info.appDir || '';
    return currentVersion;
  }

  function cacheBust(url) {
    const sep = String(url).includes('?') ? '&' : '?';
    return `${url}${sep}ddg=${Date.now()}-${Math.random().toString(16).slice(2)}`;
  }

  async function rawResponse(url, accept = '*/*') {
    const response = await fetch(cacheBust(url), {
      cache: 'no-store',
      headers: {'Accept': accept}
    });
    if (!response.ok) {
      let detail = '';
      try { detail = (await response.text()).trim(); } catch (_) {}
      throw new Error(`GitHub raw HTTP ${response.status}${detail ? ` — ${detail.slice(0, 160)}` : ''}`);
    }
    return response;
  }

  async function fetchTestSnapshot() {
    const response = await rawResponse(TEST_MANIFEST_URL, 'application/json');
    let manifest;
    try {
      manifest = JSON.parse((await response.text()).replace(/^\uFEFF/, ''));
    } catch (_) {
      throw new Error('manifest TEST invalid');
    }
    if (!manifest?.version || !/^[0-9a-f]{64}$/i.test(String(manifest?.sha256 || '').trim())) {
      throw new Error('manifest TEST incomplet');
    }
    const exeURL = /^https:\/\//i.test(String(manifest.url || '').trim())
      ? String(manifest.url).trim()
      : DEFAULT_TEST_EXE_URL;
    return {manifest, exeURL};
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
      const [{manifest, exeURL}, installed] = await Promise.all([fetchTestSnapshot(), localVersion()]);
      testState = {configured: true, manifest, exeURL, newer: isNewer(manifest.version, installed)};
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

  async function downloadVerifiedTestSnapshot(initialState) {
    let state = initialState;
    let lastMismatch = '';
    for (let attempt = 0; attempt < 3; attempt++) {
      if (!state?.manifest?.sha256 || !state?.exeURL) state = await fetchTestSnapshot();
      const response = await rawResponse(state.exeURL, 'application/octet-stream');
      const bytes = new Uint8Array(await response.arrayBuffer());
      if (bytes.byteLength > 100 * 1024 * 1024) throw new Error('build TEST prea mare (>100 MB)');
      const got = await sha256Hex(bytes);
      const expected = String(state.manifest.sha256).trim().toLowerCase();
      if (got.toLowerCase() === expected) return {state, bytes};
      lastMismatch = `${got.slice(0,12)}… != ${expected.slice(0,12)}…`;
      // A GitHub raw edge can briefly cache manifest and EXE from adjacent publish
      // commits. Re-read both instead of using api.github.com or accepting mismatch.
      await sleep(450 + attempt * 500);
      state = await fetchTestSnapshot();
    }
    throw new Error(`SHA-256 TEST diferit după reîncercări (${lastMismatch})`);
  }

  async function backendAliveForUpdate() {
    const controller = new AbortController();
    const timer = setTimeout(() => controller.abort(), 1400);
    try {
      const response = await fetch('/api/app/heartbeat', {cache:'no-store', signal:controller.signal});
      return response.ok;
    } catch (_) {
      return false;
    } finally {
      clearTimeout(timer);
    }
  }

  async function findRecoveryHelper() {
    for (const port of RECOVERY_PORTS) {
      const base = `http://127.0.0.1:${port}`;
      const controller = new AbortController();
      const timer = setTimeout(() => controller.abort(), 700);
      try {
        const response = await fetch(`${base}/status?ddg=${Date.now()}`, {cache:'no-store', signal:controller.signal});
        if (!response.ok) continue;
        const data = await response.json();
        if (!data?.ok) continue;
        if (currentAppDir && data.appDir && String(data.appDir).toLowerCase() !== String(currentAppDir).toLowerCase()) continue;
        return {base, data};
      } catch (_) {
      } finally {
        clearTimeout(timer);
      }
    }
    return null;
  }

  async function applyTestViaRecovery(manifest) {
    const helper = await findRecoveryHelper();
    if (!helper) throw new Error('recovery helper local nu răspunde');
    const response = await fetch(`${helper.base}/apply-test`, {
      method:'POST',
      headers:{'Content-Type':'application/json'},
      body:JSON.stringify({expectedVersion: manifest.version, appDir: currentAppDir || helper.data?.appDir || ''})
    });
    if (!response.ok) throw new Error((await response.text()).trim() || `recovery helper HTTP ${response.status}`);
    return response.json();
  }

  function saveVerifiedTestToDownloads(verified) {
    const manifest = verified.state.manifest;
    const blob = new Blob([verified.bytes], {type:'application/vnd.microsoft.portable-executable'});
    const href = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = href;
    a.download = `DuplicateDownloadGuard_PRO_TEST_${manifest.version}.exe`;
    a.style.display = 'none';
    document.body.appendChild(a);
    a.click();
    setTimeout(() => { URL.revokeObjectURL(href); a.remove(); }, 5000);
  }

  async function installTest(button) {
    const btn = button || document.getElementById('ddgInstallTestUpdate');
    try {
      if (!testState?.newer) await checkTest(false);
      if (!testState?.newer) {
        if (typeof window.toast === 'function') window.toast('Nu există un build TEST mai nou.');
        return;
      }
      if (!confirm(`Instalez DDG ${testState.manifest.version} din canalul TEST?\n\nEste un build de probă. Updaterul păstrează backup și face rollback automat dacă noua versiune nu pornește.`)) return;
      if (btn) {
        btn.disabled = true;
        btn.classList.add('busy');
      }

      // TEST119: if the main API already died, do not download 9+ MB into the
      // stale Edge renderer only to discover that /api/update/apply is gone.
      // The independent recovery helper verifies/downloads the official build
      // itself and can replace/restart DDG even while this window says OFFLINE.
      if (!(await backendAliveForUpdate())) {
        if (btn) btn.textContent = 'Recovery TEST…';
        try {
          const recovered = await applyTestViaRecovery(testState.manifest);
          const status = document.getElementById('ddgTestUpdateStatus');
          if (status) status.textContent = `${recovered.message || 'Recovery helper aplică update-ul.'} Fereastra se va redeschide.`;
          if (typeof window.toast === 'function') window.toast('Backend OFFLINE: recovery helper aplică update-ul TEST.');
          return;
        } catch (recoveryErr) {
          if (btn) btn.textContent = 'Descarc fallback…';
          const verifiedFallback = await downloadVerifiedTestSnapshot(testState);
          saveVerifiedTestToDownloads(verifiedFallback);
          throw new Error(`${recoveryErr.message}. Am salvat EXE-ul TEST verificat în Downloads ca fallback.`);
        }
      }

      if (btn) btn.textContent = 'Descarc TEST…';
      const verified = await downloadVerifiedTestSnapshot(testState);
      const manifest = verified.state.manifest;
      testState = {...testState, ...verified.state};
      if (btn) btn.textContent = 'Aplic TEST…';
      const form = new FormData();
      form.append('file', new Blob([verified.bytes], {type: 'application/vnd.microsoft.portable-executable'}), `DuplicateDownloadGuard_TEST_${manifest.version}.exe`);
      try {
        const apply = await fetch('/api/update/apply', {method: 'POST', body: form});
        if (!apply.ok) throw new Error((await apply.text()).trim() || `updater local HTTP ${apply.status}`);
        await apply.json();
      } catch (localApplyErr) {
        if (btn) btn.textContent = 'Recovery TEST…';
        try {
          await applyTestViaRecovery(manifest);
          if (typeof window.toast === 'function') window.toast('Updaterul principal nu a răspuns; recovery helper continuă update-ul.');
          return;
        } catch (recoveryErr) {
          saveVerifiedTestToDownloads(verified);
          throw new Error(`${localApplyErr.message} | recovery: ${recoveryErr.message}. EXE-ul TEST verificat a fost salvat în Downloads.`);
        }
      }
    } catch (err) {
      if (typeof window.toast === 'function') window.toast('Update TEST: ' + err.message);
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