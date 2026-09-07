// TEST v8.5.67 — deterministic JDownloader FlashGot handoff.
// Uses CURRENT DDG results only: no /api/download/preflight and no HDD rescan.
// One explicit form POST to JDExternInterface /flashgot = one handoff request.
(() => {
  'use strict';

  const JD_BASE = 'http://127.0.0.1:9666';
  let busy = false;
  let pending = null;

  const liveEngine = () => String(document.getElementById('downloadMethod')?.value || window.cfg?.downloadMethod || '').trim().toLowerCase();

  function selectedIDs() {
    try {
      return typeof window.idsForAction === 'function'
        ? window.idsForAction().map(Number).filter(Number.isFinite)
        : [];
    } catch (_) { return []; }
  }

  async function rowsForIDs(ids) {
    const unique = [...new Set((ids || []).map(Number).filter(Number.isFinite))];
    if (!unique.length) return [];
    const wanted = new Set(unique);
    const rows = [];
    let offset = 0;
    for (let page = 0; page < 250 && wanted.size; page++) {
      const data = await window.api(`/api/results?offset=${offset}&limit=1000&status=ALL&sort=path&order=asc`);
      const batch = Array.isArray(data?.rows) ? data.rows : [];
      for (const row of batch) {
        const id = Number(row?.id);
        if (wanted.has(id)) { rows.push(row); wanted.delete(id); }
      }
      if (!batch.length || offset + batch.length >= Number(data?.total || 0)) break;
      offset += batch.length;
    }
    if (wanted.size) throw new Error(`Nu mai găsesc ${wanted.size} rezultat(e) selectate în scanarea curentă.`);
    const order = new Map(unique.map((id, i) => [id, i]));
    rows.sort((a, b) => (order.get(Number(a.id)) ?? 0) - (order.get(Number(b.id)) ?? 0));
    return rows;
  }

  function stableProviderURL(row) {
    const remote = row?.remote || {};
    const source = String(remote.source || '').trim().toUpperCase();
    try {
      const origin = new URL(remote.url || '').origin;
      if (source === 'GOFILE' && remote.providerId) {
        const u = new URL(remote.url || '');
        const parts = u.pathname.split('/').filter(Boolean);
        if (parts.length >= 2 && parts[0].toLowerCase() === 'd') {
          return `https://gofile.io/?c=${encodeURIComponent(parts[1])}#file=${encodeURIComponent(String(remote.providerId))}`;
        }
      }
      if (source === 'BUNKR' && remote.handle) return `${origin}/f/${encodeURIComponent(String(remote.handle))}`;
      if (source === 'CYBERDROP' && remote.providerId) return `${origin}/f/${encodeURIComponent(String(remote.providerId))}`;
    } catch (_) {}
    return '';
  }

  function jdURL(row) {
    const remote = row?.remote || {};
    return stableProviderURL(row) || String(remote.directUrl || remote.url || '').trim();
  }

  function cleanPackageName(value) {
    return String(value || '').replace(/[<>:"/\\|?*\u0000-\u001f]/g, ' ').replace(/\s+/g, ' ').trim().slice(0, 120);
  }

  function onePackageName(rows) {
    const paths = (rows || []).map(r => String(r?.remote?.path || '').replace(/\\/g, '/')).filter(Boolean);
    const parents = paths.map(p => p.split('/').filter(Boolean).slice(0, -1));
    if (parents.length && parents.every(p => p.length)) {
      const first = parents[0];
      const common = [];
      for (let i = 0; i < first.length; i++) {
        if (parents.every(p => p[i] === first[i])) common.push(first[i]);
        else break;
      }
      const commonName = cleanPackageName(common.join(' - '));
      if (commonName) return commonName;
    }
    const first = rows?.[0]?.remote || {};
    const provider = String(first.source || 'DDG').trim().toUpperCase() || 'DDG';
    try {
      const u = new URL(first.url || '');
      const parts = u.pathname.split('/').filter(Boolean);
      const tail = decodeURIComponent(parts.at(-1) || u.hostname || 'selecție');
      const name = cleanPackageName(`${provider} - ${tail}`);
      if (name) return name;
    } catch (_) {}
    return 'DDG - selecție';
  }

  function commonSourceURL(rows) {
    const sources = [...new Set((rows || []).map(row => String(row?.remote?.url || '').trim()).filter(Boolean))];
    return sources.length === 1 ? sources[0] : '';
  }

  function buildSubmission(rows) {
    const urls = [];
    const descriptions = [];
    const filenames = [];
    const seen = new Set();
    for (const row of rows || []) {
      const link = jdURL(row);
      if (!link || seen.has(link)) continue;
      seen.add(link);
      urls.push(link);
      const filename = String(row?.remote?.name || row?.remote?.path || 'DDG').trim() || 'DDG';
      descriptions.push(filename);
      filenames.push(filename);
    }
    if (!urls.length) throw new Error('Selecția nu conține linkuri compatibile cu JDownloader.');
    const packageName = onePackageName(rows);
    const params = new URLSearchParams();
    params.set('urls', urls.join('\n'));
    // Current JDownloader ExternInterfaceImpl.flashgot() reads plural
    // "descriptions", plus fnames/referer/package. Keep this aligned with JD.
    params.set('descriptions', descriptions.join('\n'));
    params.set('fnames', filenames.join('\n'));
    params.set('package', packageName);
    const referer = commonSourceURL(rows);
    if (referer) params.set('referer', referer);
    return {params, count: urls.length, packageName, referer};
  }

  async function checkJD() {
    return await new Promise(resolve => {
      document.getElementById('ddgJDCheckV8567')?.remove();
      window.jdownloader = false;
      const script = document.createElement('script');
      script.id = 'ddgJDCheckV8567';
      script.src = `${JD_BASE}/jdcheck.js?_ddg=${Date.now()}`;
      let done = false;
      const finish = running => {
        if (done) return;
        done = true;
        script.remove();
        resolve(Boolean(running));
      };
      script.onload = () => finish(window.jdownloader === true);
      script.onerror = () => finish(false);
      document.head.appendChild(script);
      setTimeout(() => finish(window.jdownloader === true), 2200);
    });
  }

  function submitOneForm(params) {
    // Never fetch() then retry: if a client blocks reading the cross-origin
    // response after JD accepted the POST, retrying would duplicate the links.
    const frameName = `ddgJDOneShot_${Date.now()}_${Math.random().toString(36).slice(2)}`;
    const iframe = document.createElement('iframe');
    iframe.name = frameName;
    iframe.style.display = 'none';
    document.body.appendChild(iframe);
    const form = document.createElement('form');
    form.method = 'POST';
    form.action = `${JD_BASE}/flashgot`;
    form.target = frameName;
    form.style.display = 'none';
    for (const [name, value] of params.entries()) {
      const input = document.createElement('input');
      input.type = 'hidden';
      input.name = name;
      input.value = value;
      form.appendChild(input);
    }
    document.body.appendChild(form);
    form.submit();
    setTimeout(() => { form.remove(); iframe.remove(); }, 5000);
  }

  function classify(row) {
    // guardVerdict is DDG's final authoritative decision and is the same verdict
    // shown by Smart Guard in the main results table. Do not let an older manual
    // or auto status reclassify it inside the JDownloader popup.
    const guard = String(row?.guardVerdict || '').trim().toUpperCase();
    if (['DOWNLOAD','DUPLICATE','REVIEW'].includes(guard)) return guard;

    // Legacy fallback only for rows that have no final Guard verdict yet.
    const manual = Boolean(row?.manual);
    const status = String(row?.status || row?.autoStatus || '').trim().toUpperCase();
    if (manual && ['HAVE','VERIFIED'].includes(status)) return 'DUPLICATE';
    if (['HAVE','VERIFIED'].includes(status)) return 'DUPLICATE';
    if (['POSSIBLE','SAMPLED','REVIEW','UNKNOWN',''].includes(status)) return 'REVIEW';
    if (['MISSING','DIFFERENT','DIFF'].includes(status)) return 'DOWNLOAD';
    return manual ? 'REVIEW' : 'DOWNLOAD';
  }

  function splitRows(rows) {
    const groups = {DOWNLOAD:[], DUPLICATE:[], REVIEW:[]};
    for (const row of rows || []) groups[classify(row)].push(row);
    return groups;
  }

  async function submitRows(rows) {
    if (!(await checkJD())) throw new Error('JDownloader 2 nu răspunde pe 127.0.0.1:9666. Verifică dacă JD este pornit și External Interface/FlashGot este activ.');
    const submission = buildSubmission(rows);
    submitOneForm(submission.params);
    return submission;
  }

  function installDialog() {
    if (document.getElementById('ddgJDFastDecisionV8567')) return;
    const style = document.createElement('style');
    style.id = 'ddgJDFastDecisionV8567Style';
    style.textContent = `
      #ddgJDFastDecisionV8567{position:fixed;inset:0;z-index:12050;background:rgba(3,7,12,.80);display:flex;align-items:center;justify-content:center;padding:24px}
      #ddgJDFastDecisionV8567.hidden{display:none}
      #ddgJDFastDecisionV8567 .box{width:min(720px,94vw);background:#0e1721;border:1px solid #32465a;border-radius:14px;box-shadow:0 24px 70px rgba(0,0,0,.45);overflow:hidden}
      #ddgJDFastDecisionV8567 .head{padding:16px 18px;border-bottom:1px solid #26394b;font-size:17px;font-weight:800}
      #ddgJDFastDecisionV8567 .body{padding:16px 18px;color:#c7d7e7;line-height:1.5}
      #ddgJDFastDecisionV8567 .summary{display:grid;grid-template-columns:repeat(3,1fr);gap:9px;margin:13px 0}
      #ddgJDFastDecisionV8567 .card{border:1px solid #2c4053;border-radius:10px;padding:11px;background:#0a121a}
      #ddgJDFastDecisionV8567 .card b{display:block;font-size:20px;margin-bottom:2px}
      #ddgJDFastDecisionV8567 .note{padding:10px 12px;border-left:3px solid #65b7ff;background:#102030;border-radius:8px;font-size:12px}
      #ddgJDFastDecisionV8567 .foot{display:flex;gap:9px;justify-content:flex-end;flex-wrap:wrap;padding:13px 18px;border-top:1px solid #26394b}
      @media(max-width:620px){#ddgJDFastDecisionV8567 .summary{grid-template-columns:1fr}}
    `;
    document.head.appendChild(style);
    document.body.insertAdjacentHTML('beforeend', `
      <div id="ddgJDFastDecisionV8567" class="hidden" role="dialog" aria-modal="true">
        <div class="box">
          <div class="head">JDownloader — fără rescanare HDD</div>
          <div class="body">
            <div id="ddgJDFastTextV8567"></div>
            <div class="summary">
              <div class="card"><b id="ddgJDFastDownloadV8567">0</b>Recomandate</div>
              <div class="card"><b id="ddgJDFastDuplicateV8567">0</b>Ai deja</div>
              <div class="card"><b id="ddgJDFastReviewV8567">0</b>De verificat</div>
            </div>
            <div class="note">Folosesc exclusiv rezultatele scanării curente. Nu pornesc /api/download/preflight și nu rescanez HDD-urile. Trimiterea către JD este un singur POST FlashGot cu un singur nume de pachet.</div>
          </div>
          <div class="foot">
            <button class="btn" type="button" id="ddgJDFastCancelV8567">Anulează</button>
            <button class="btn" type="button" id="ddgJDFastRecommendedV8567">Trimite recomandate</button>
            <button class="btn primary" type="button" id="ddgJDFastAllV8567">Trimite TOATE</button>
          </div>
        </div>
      </div>`);
    document.getElementById('ddgJDFastCancelV8567')?.addEventListener('click', closeDialog);
    document.getElementById('ddgJDFastRecommendedV8567')?.addEventListener('click', async () => {
      if (pending?.recommended?.length) await sendRowsNow(pending.recommended);
    });
    document.getElementById('ddgJDFastAllV8567')?.addEventListener('click', async () => {
      if (pending?.all?.length) await sendRowsNow(pending.all);
    });
  }

  function closeDialog() { document.getElementById('ddgJDFastDecisionV8567')?.classList.add('hidden'); }

  function showDialog(rows) {
    installDialog();
    const groups = splitRows(rows);
    pending = {all:rows.slice(), recommended:groups.DOWNLOAD.slice(), groups};
    document.getElementById('ddgJDFastTextV8567').innerHTML = `<b>${rows.length} fișier(e) selectate.</b> Alege exact ce trimiți în JDownloader.`;
    document.getElementById('ddgJDFastDownloadV8567').textContent = String(groups.DOWNLOAD.length);
    document.getElementById('ddgJDFastDuplicateV8567').textContent = String(groups.DUPLICATE.length);
    document.getElementById('ddgJDFastReviewV8567').textContent = String(groups.REVIEW.length);
    const recommended = document.getElementById('ddgJDFastRecommendedV8567');
    const all = document.getElementById('ddgJDFastAllV8567');
    recommended.disabled = groups.DOWNLOAD.length === 0;
    recommended.textContent = `Trimite recomandate (${groups.DOWNLOAD.length})`;
    all.disabled = rows.length === 0;
    all.textContent = `Trimite TOATE (${rows.length})`;
    document.getElementById('ddgJDFastDecisionV8567')?.classList.remove('hidden');
  }

  async function sendRowsNow(rows) {
    if (busy || !rows?.length) return;
    busy = true;
    try {
      const result = await submitRows(rows);
      closeDialog();
      pending = null;
      window.toast?.(`JDownloader: ${result.count} fișier(e) • un singur pachet „${result.packageName}”`);
    } catch (error) { window.toast?.(error?.message || String(error)); }
    finally { busy = false; }
  }

  async function sendBatchAware() {
    if (busy) return;
    const ids = selectedIDs();
    if (!ids.length) return window.toast?.('Selectează fișiere');
    busy = true;
    const button = document.getElementById('downloadGuardBtn') || document.querySelector('button[onclick="downloadSelected()"]');
    if (button) { button.disabled = true; button.textContent = '⏳ Citesc rezultatele curente…'; }
    try { showDialog(await rowsForIDs(ids)); }
    catch (error) { window.toast?.(error?.message || String(error)); }
    finally {
      busy = false;
      if (button) { button.disabled = false; button.textContent = liveEngine() === 'jdownloader' ? '⬇ Trimite în JDownloader' : '⬇ Descarcă'; }
    }
  }

  async function sendExactIDs(ids, options = {}) {
    if (busy) return;
    const unique = [...new Set((ids || []).map(Number).filter(Number.isFinite))];
    if (!unique.length) return window.toast?.('Nu există fișiere de trimis');
    busy = true;
    try {
      const rows = await rowsForIDs(unique);
      const groups = splitRows(rows);
      if (options.confirm !== false) {
        const ok = window.confirm(`Trimit ${rows.length} fișier(e) într-un singur pachet JDownloader?\n\nRecomandate: ${groups.DOWNLOAD.length}\nAi deja: ${groups.DUPLICATE.length}\nDe verificat: ${groups.REVIEW.length}\n\nNu se face nicio rescanare HDD.`);
        if (!ok) return;
      }
      const result = await submitRows(rows);
      window.toast?.(`JDownloader: ${result.count} fișier(e) • un singur pachet „${result.packageName}”`);
      return result;
    } catch (error) {
      window.toast?.(error?.message || String(error));
      throw error;
    } finally { busy = false; }
  }

  function install() {
    const select = document.getElementById('downloadMethod');
    const button = document.getElementById('downloadGuardBtn') || document.querySelector('button[onclick="downloadSelected()"]');
    if (button && liveEngine() === 'jdownloader' && !busy) {
      button.textContent = '⬇ Trimite în JDownloader';
      button.title = 'Folosește rezultatele curente; fără rescanare HDD. O singură trimitere FlashGot / un singur pachet.';
    }
    if (select && !select.dataset.ddgFastJDBoundV8567) {
      select.dataset.ddgFastJDBoundV8567 = '1';
      select.addEventListener('change', () => setTimeout(install, 0));
    }
    window.sendSelectedJD2 = sendBatchAware;
  }

  // Kept as a second safety net. The window-capture guard loads first and owns
  // real JD clicks before any legacy document-capture listener can run.
  document.addEventListener('click', event => {
    const target = event.target?.closest?.('#jdSelectedBtn, button[onclick="sendSelectedJD2()"], #downloadGuardBtn, button[onclick="downloadSelected()"]');
    if (!target) return;
    const explicitJD = target.id === 'jdSelectedBtn' || target.getAttribute('onclick') === 'sendSelectedJD2()';
    const primaryJD = (target.id === 'downloadGuardBtn' || target.getAttribute('onclick') === 'downloadSelected()') && liveEngine() === 'jdownloader';
    if (!explicitJD && !primaryJD) return;
    event.preventDefault();
    event.stopImmediatePropagation();
    sendBatchAware();
  }, true);

  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', install, {once:true});
  else install();
  setTimeout(install, 400);
  setTimeout(install, 1200);

  const api = {sendBatchAware, sendExactIDs, rowsForIDs, onePackageName, buildSubmission};
  window.ddgJDownloaderFastV8567 = api;
  // Compatibility alias for Media Picker/window guard created in the same TEST series.
  window.ddgJDownloaderFastV8566 = api;
})();
