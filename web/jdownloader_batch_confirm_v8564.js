// Fast, batch-aware JDownloader handoff for TEST builds.
// Default flow reuses the CURRENT DDG result set instead of walking hundreds of
// thousands of local files again. A full live HDD recheck remains available as
// an explicit option. Nothing is sent until the user chooses recommended or ALL.
(() => {
  'use strict';

  const JD_BASE = 'http://127.0.0.1:9666';
  let busy = false;
  let pending = null;

  const liveEngine = () => String(document.getElementById('downloadMethod')?.value || '').trim().toLowerCase();
  const selectedIDs = () => {
    try {
      return typeof window.idsForAction === 'function'
        ? window.idsForAction().map(Number).filter(Number.isFinite)
        : [];
    } catch (_) { return []; }
  };
  const guardDestination = () => String(document.getElementById('downloadDir')?.value || window.cfg?.downloadDir || '').trim();
  const guardMode = () => document.getElementById('downloadGuardMode')?.value || window.cfg?.downloadGuardMode || 'smart';

  async function rowsForIDs(ids) {
    const wanted = new Set(ids.map(Number));
    const rows = [];
    let offset = 0;
    for (let page = 0; page < 100 && wanted.size; page++) {
      const data = await window.api(`/api/results?offset=${offset}&limit=1000&status=ALL`);
      const batch = Array.isArray(data?.rows) ? data.rows : [];
      for (const row of batch) {
        const id = Number(row?.id);
        if (wanted.has(id)) {
          rows.push(row);
          wanted.delete(id);
        }
      }
      if (!batch.length || offset + batch.length >= Number(data?.total || 0)) break;
      offset += batch.length;
    }
    if (wanted.size) throw new Error(`Nu mai găsesc ${wanted.size} rezultat(e) selectate.`);
    return rows;
  }

  function gofileURL(row) {
    const r = row?.remote || {};
    if (String(r.source || '').toUpperCase() !== 'GOFILE' || !r.providerId) return '';
    try {
      const u = new URL(r.url || '');
      const parts = u.pathname.split('/').filter(Boolean);
      if (parts.length >= 2 && parts[0].toLowerCase() === 'd') {
        return `https://gofile.io/?c=${encodeURIComponent(parts[1])}#file=${encodeURIComponent(r.providerId)}`;
      }
    } catch (_) {}
    return '';
  }

  function bunkrURL(row) {
    const r = row?.remote || {};
    if (String(r.source || '').toUpperCase() !== 'BUNKR' || !r.handle) return '';
    try {
      const album = new URL(r.url || '');
      return `${album.origin}/f/${encodeURIComponent(String(r.handle))}`;
    } catch (_) { return ''; }
  }

  function cyberdropURL(row) {
    const r = row?.remote || {};
    if (String(r.source || '').toUpperCase() !== 'CYBERDROP' || !r.providerId) return '';
    try {
      const album = new URL(r.url || '');
      return `${album.origin}/f/${encodeURIComponent(String(r.providerId))}`;
    } catch (_) { return ''; }
  }

  function jdURL(row) {
    const r = row?.remote || {};
    return gofileURL(row) || bunkrURL(row) || cyberdropURL(row) || String(r.directUrl || r.url || '').trim();
  }

  function cleanPackageName(value) {
    return String(value || '')
      .replace(/[<>:"/\\|?*\u0000-\u001f]/g, ' ')
      .replace(/\s+/g, ' ')
      .trim()
      .slice(0, 120);
  }

  function onePackageName(rows) {
    const paths = rows.map(r => String(r?.remote?.path || '').replace(/\\/g, '/')).filter(Boolean);
    const parents = paths.map(p => p.split('/').filter(Boolean).slice(0, -1));
    if (parents.length && parents.every(p => p.length)) {
      const first = parents[0];
      const common = [];
      for (let i = 0; i < first.length; i++) {
        if (parents.every(p => p[i] === first[i])) common.push(first[i]);
        else break;
      }
      if (common.length) {
        const name = cleanPackageName(common.join(' - '));
        if (name) return name;
      }
    }
    const first = rows[0]?.remote || {};
    const provider = String(first.source || 'DDG').toUpperCase();
    try {
      const u = new URL(first.url || '');
      const parts = u.pathname.split('/').filter(Boolean);
      const id = parts.length ? parts[parts.length - 1] : u.hostname;
      const name = cleanPackageName(`${provider} - ${decodeURIComponent(id)}`);
      if (name) return name;
    } catch (_) {}
    return 'DDG - selecție';
  }

  async function checkJD() {
    if (window.ddgDownloadActionsV8545?.checkJDownloader) {
      return window.ddgDownloadActionsV8545.checkJDownloader();
    }
    return await new Promise(resolve => {
      document.getElementById('ddgBatchJDCheck')?.remove();
      window.jdownloader = false;
      const s = document.createElement('script');
      s.id = 'ddgBatchJDCheck';
      s.src = `${JD_BASE}/jdcheck.js?_ddg=${Date.now()}`;
      let done = false;
      const finish = running => {
        if (done) return;
        done = true;
        s.remove();
        resolve({running: Boolean(running)});
      };
      s.onload = () => finish(window.jdownloader === true);
      s.onerror = () => finish(false);
      document.head.appendChild(s);
      setTimeout(() => finish(window.jdownloader === true), 2200);
    });
  }

  function buildSubmission(rows) {
    const urls = [];
    const descriptions = [];
    const seen = new Set();
    for (const row of rows) {
      const url = jdURL(row);
      if (!url || seen.has(url)) continue;
      seen.add(url);
      urls.push(url);
      descriptions.push(row?.remote?.name || row?.remote?.path || 'DDG');
    }
    if (!urls.length) throw new Error('Selecția nu conține linkuri compatibile cu JDownloader.');
    const packageName = onePackageName(rows);
    const params = new URLSearchParams();
    params.set('urls', urls.join('\n'));
    params.set('descriptions', descriptions.join('\n'));
    params.set('package', packageName);
    // One FlashGot request = one JD package. Do not force dir or autostart.
    return {params, count: urls.length, packageName};
  }

  function submitHiddenForm(params) {
    const frameName = `ddgJDBatch_${Date.now()}`;
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
    setTimeout(() => { form.remove(); iframe.remove(); }, 4000);
  }

  async function submitRows(rows) {
    const jd = await checkJD();
    if (!jd?.running) throw new Error('JDownloader 2 nu răspunde pe 127.0.0.1:9666.');
    const submission = buildSubmission(rows);
    try {
      const response = await fetch(`${JD_BASE}/flashgot`, {
        method:'POST',
        headers:{'Content-Type':'application/x-www-form-urlencoded;charset=UTF-8'},
        body:submission.params.toString(),
        cache:'no-store'
      });
      const reply = (await response.text()).trim();
      if (!response.ok || /(^|\s)failed(\s|$)/i.test(reply)) throw new Error(reply || `HTTP ${response.status}`);
      return {...submission, confirmed:true};
    } catch (_) {
      submitHiddenForm(submission.params);
      return {...submission, confirmed:false};
    }
  }

  async function preflight(ids, dest, mode) {
    return window.api('/api/download/preflight', {
      method:'POST',
      headers:{'Content-Type':'application/json'},
      body:JSON.stringify({ids, destination:dest, mode})
    });
  }

  function classifyCurrentRow(row) {
    const manual = Boolean(row?.manual);
    const status = String(row?.status || row?.autoStatus || '').trim().toUpperCase();
    const guard = String(row?.guardVerdict || '').trim().toUpperCase();

    // Manual HAVE must never disappear from the confirmation just because an
    // older automatic verdict said DOWNLOAD.
    if (manual && ['HAVE','VERIFIED'].includes(status)) return 'DUPLICATE';
    if (guard === 'DUPLICATE') return 'DUPLICATE';
    if (guard === 'REVIEW') return 'REVIEW';
    if (guard === 'DOWNLOAD' && !manual) return 'DOWNLOAD';

    if (['HAVE','VERIFIED'].includes(status)) return 'DUPLICATE';
    if (['POSSIBLE','SAMPLED','REVIEW','UNKNOWN',''].includes(status)) return 'REVIEW';
    if (['MISSING','DIFFERENT','DIFF'].includes(status)) return 'DOWNLOAD';
    return manual ? 'REVIEW' : 'DOWNLOAD';
  }

  function currentReport(rows) {
    const counts = {DOWNLOAD:0, DUPLICATE:0, REVIEW:0};
    const decisions = rows.map(row => {
      const verdict = classifyCurrentRow(row);
      counts[verdict]++;
      return {
        resultId:Number(row.id),
        name:row?.remote?.name || row?.remote?.path || '',
        verdict,
        reason: verdict === 'DOWNLOAD'
          ? 'Rezultatul curent DDG îl marchează ca lipsă.'
          : verdict === 'DUPLICATE'
            ? 'Rezultatul curent DDG/manual indică faptul că există deja.'
            : 'Rezultatul curent necesită verificare.'
      };
    });
    return {mode:'current-results', decisions, counts, durationMs:0, fastCurrentResults:true};
  }

  function idsByVerdict(report, verdict) {
    return (report?.decisions || [])
      .filter(x => String(x?.verdict || '').toUpperCase() === verdict)
      .map(x => Number(x.resultId))
      .filter(Number.isFinite);
  }

  function installDecisionModal() {
    if (document.getElementById('ddgJDBatchDecision')) return;
    const style = document.createElement('style');
    style.id = 'ddgJDBatchDecisionStyle';
    style.textContent = `
      #ddgJDBatchDecision{position:fixed;inset:0;z-index:10150;background:rgba(3,7,12,.78);display:flex;align-items:center;justify-content:center;padding:24px}
      #ddgJDBatchDecision.hidden{display:none}
      #ddgJDBatchDecision .box{width:min(760px,94vw);background:#0e1721;border:1px solid #32465a;border-radius:14px;box-shadow:0 24px 70px rgba(0,0,0,.45);overflow:hidden}
      #ddgJDBatchDecision .head{padding:16px 18px;border-bottom:1px solid #26394b;font-size:17px;font-weight:800}
      #ddgJDBatchDecision .body{padding:16px 18px;color:#c7d7e7;line-height:1.5}
      #ddgJDBatchDecision .summary{display:grid;grid-template-columns:repeat(3,1fr);gap:9px;margin:13px 0}
      #ddgJDBatchDecision .card{border:1px solid #2c4053;border-radius:10px;padding:11px;background:#0a121a}
      #ddgJDBatchDecision .card b{display:block;font-size:20px;margin-bottom:2px}
      #ddgJDBatchDecision .note{padding:10px 12px;border-left:3px solid #65b7ff;background:#102030;border-radius:8px;font-size:12px}
      #ddgJDBatchDecision .fullwarn{margin-top:9px;color:#d4b777;font-size:11px}
      #ddgJDBatchDecision .foot{display:flex;gap:9px;justify-content:flex-end;flex-wrap:wrap;padding:13px 18px;border-top:1px solid #26394b}
      @media(max-width:620px){#ddgJDBatchDecision .summary{grid-template-columns:1fr}}
    `;
    document.head.appendChild(style);
    document.body.insertAdjacentHTML('beforeend', `
      <div id="ddgJDBatchDecision" class="hidden" role="dialog" aria-modal="true">
        <div class="box">
          <div class="head">Confirmare JDownloader</div>
          <div class="body">
            <div id="ddgJDBatchDecisionText"></div>
            <div class="summary">
              <div class="card"><b id="ddgJDSafeCount">0</b>Recomandate</div>
              <div class="card"><b id="ddgJDHaveCount">0</b>Ai deja</div>
              <div class="card"><b id="ddgJDReviewCount">0</b>De verificat</div>
            </div>
            <div class="note" id="ddgJDBatchNote"></div>
            <div class="fullwarn">„Reverifică HDD complet” face intenționat o scanare live a tuturor locațiilor și poate dura multe minute pe colecții foarte mari.</div>
          </div>
          <div class="foot">
            <button class="btn" type="button" id="ddgJDCancel">Anulează</button>
            <button class="btn" type="button" id="ddgJDFullRecheck">Reverifică HDD complet</button>
            <button class="btn" type="button" id="ddgJDSendSafe">Trimite recomandate</button>
            <button class="btn primary" type="button" id="ddgJDSendAll">Trimite TOATE</button>
          </div>
        </div>
      </div>`);
    document.getElementById('ddgJDCancel')?.addEventListener('click', closeDecision);
    document.getElementById('ddgJDSendSafe')?.addEventListener('click', sendSafeOnly);
    document.getElementById('ddgJDSendAll')?.addEventListener('click', confirmAll);
    document.getElementById('ddgJDFullRecheck')?.addEventListener('click', fullRecheck);
    document.getElementById('ddgJDBatchDecision')?.addEventListener('click', e => {
      if (e.target?.id === 'ddgJDBatchDecision') closeDecision();
    });
  }

  function closeDecision() {
    document.getElementById('ddgJDBatchDecision')?.classList.add('hidden');
  }

  function showDecision(report, ids, rows, sourceLabel) {
    installDecisionModal();
    const safe = idsByVerdict(report, 'DOWNLOAD');
    const review = idsByVerdict(report, 'REVIEW');
    const duplicates = idsByVerdict(report, 'DUPLICATE');
    pending = {all:ids.slice(), safe, review, duplicates, rows:rows.slice(), report};

    const text = document.getElementById('ddgJDBatchDecisionText');
    const note = document.getElementById('ddgJDBatchNote');
    const safeEl = document.getElementById('ddgJDSafeCount');
    const dupEl = document.getElementById('ddgJDHaveCount');
    const revEl = document.getElementById('ddgJDReviewCount');
    if (safeEl) safeEl.textContent = String(safe.length);
    if (dupEl) dupEl.textContent = String(duplicates.length);
    if (revEl) revEl.textContent = String(review.length);
    if (text) text.innerHTML = `<b>${ids.length} fișiere selectate.</b> DDG nu trimite nimic până nu confirmi.`;
    if (note) note.textContent = sourceLabel === 'full'
      ? `Reverificarea live s-a terminat. Au fost verificate ${Number(report?.scannedFiles || 0).toLocaleString('ro-RO')} fișiere locale în ${(Number(report?.durationMs || 0)/1000).toFixed(1)} secunde.`
      : 'Verificare rapidă: folosesc rezultatele deja obținute în scanarea curentă. NU rescanez cele sute de mii de fișiere de pe HDD.';

    const safeBtn = document.getElementById('ddgJDSendSafe');
    const allBtn = document.getElementById('ddgJDSendAll');
    if (safeBtn) {
      safeBtn.disabled = safe.length === 0;
      safeBtn.textContent = `Trimite recomandate (${safe.length})`;
    }
    if (allBtn) {
      allBtn.disabled = ids.length === 0;
      allBtn.textContent = `Trimite TOATE (${ids.length})`;
    }
    document.getElementById('ddgJDBatchDecision')?.classList.remove('hidden');
  }

  async function sendIDs(ids) {
    if (busy || !ids.length) return;
    busy = true;
    try {
      const rows = pending?.rows?.length
        ? pending.rows.filter(r => ids.includes(Number(r.id)))
        : await rowsForIDs(ids);
      const result = await submitRows(rows);
      closeDecision();
      window.closeGuardReport?.();
      window.toast?.(`JDownloader: ${result.count} fișier(e) într-un singur pachet „${result.packageName}”`);
      pending = null;
    } catch (error) {
      window.toast?.(error?.message || String(error));
    } finally {
      busy = false;
    }
  }

  async function confirmAll() {
    if (!pending?.all?.length || busy) return;
    const blocked = pending.duplicates.length;
    const review = pending.review.length;
    const ok = window.confirm(
      `Confirmi trimiterea TUTUROR celor ${pending.all.length} fișiere în JDownloader?\n\nDDG indică ${blocked} „ai deja” și ${review} de verificat. Acestea vor fi trimise și ele.`
    );
    if (!ok) return;
    await sendIDs(pending.all);
  }

  async function sendSafeOnly() {
    if (!pending?.safe?.length || busy) return;
    await sendIDs(pending.safe);
  }

  async function fullRecheck() {
    if (!pending?.all?.length || busy) return;
    busy = true;
    const btn = document.getElementById('ddgJDFullRecheck');
    if (btn) {
      btn.disabled = true;
      btn.textContent = '⏳ Scanez HDD…';
    }
    try {
      const report = await preflight(pending.all, guardDestination(), guardMode());
      showDecision(report, pending.all, pending.rows, 'full');
    } catch (error) {
      window.toast?.(error?.message || String(error));
    } finally {
      busy = false;
      if (btn) {
        btn.disabled = false;
        btn.textContent = 'Reverifică HDD complet';
      }
    }
  }

  async function sendBatchAware() {
    if (busy) return;
    const ids = selectedIDs();
    if (!ids.length) return window.toast?.('Selectează fișiere');
    const button = document.getElementById('downloadGuardBtn') || document.querySelector('button[onclick="downloadSelected()"]');
    busy = true;
    if (button) {
      button.disabled = true;
      button.textContent = '⏳ Pregătesc confirmarea…';
    }
    try {
      // Deliberately do NOT call /api/download/preflight here. The user already
      // has a current result set; walking 300k+ files a second time made this
      // action take 10+ minutes. Full live recheck is available in the modal.
      const rows = await rowsForIDs(ids);
      const report = currentReport(rows);
      showDecision(report, ids, rows, 'current');
    } catch (error) {
      window.toast?.(error?.message || String(error));
    } finally {
      busy = false;
      if (button) {
        button.disabled = false;
        button.textContent = liveEngine() === 'jdownloader' ? '⬇ Verifică și trimite în JDownloader' : '⬇ Descarcă';
      }
    }
  }

  // Capture before legacy handlers. JDownloader always uses this confirmation
  // flow; integrated DDG downloads keep their existing guard behavior.
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

  setTimeout(() => { window.sendSelectedJD2 = sendBatchAware; }, 1000);

  window.ddgJDownloaderBatchConfirmV8564 = {sendBatchAware, confirmAll, sendSafeOnly, fullRecheck, onePackageName};
})();
