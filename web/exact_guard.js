// ExactGuard v8.5 UI layer. Internal verdicts remain compatible with older
// queue/update code, while the interface shows short human-facing statuses.
(() => {
  'use strict';

  let lastRequest = null;
  let lastReport = null;
  let guardTicker = null;

  function showActivity(text, kind = 'info') {
    const el = document.getElementById('guardActivity');
    if (!el) return;
    el.classList.remove('hidden');
    el.style.borderLeftColor = kind === 'error' ? '#ff6b7a' : kind === 'ok' ? '#3ddc97' : '#4da3ff';
    el.textContent = text;
  }

  const guardModeLabel = mode => ({
    smart: 'Smart Guard',
    exact: 'Exact Guard',
    ai: 'AI Guard'
  })[mode] || 'Smart Guard';

  const legacyVerdictLabel = verdict => ({
    DOWNLOAD: 'NU ÎL AI',
    DUPLICATE: 'AI DEJA',
    REVIEW: 'POSIBIL DUPLICAT'
  })[verdict] || verdict;

  function inferUserStatus(item) {
    if (!item) return 'NECUNOSCUT';
    if (item.userStatus) return item.userStatus;
    const method = item.method || item.guardMethod || '';
    if (method === 'download-history') return 'DESCĂRCAT DEJA';
    if (method === 'media-same-content') return 'ACELAȘI CONȚINUT';
    if (method === 'media-version') return 'ALTĂ VERSIUNE';
    if (method === 'media-looks-same' || method === 'deterministic-samples') return 'PARE ACELAȘI';
    if (['metadata-incomplete', 'mega-busy', 'remote-unavailable', 'full-sha256-error', 'sample-error'].includes(method)) return 'NU S-A PUTUT VERIFICA';
    return legacyVerdictLabel(item.verdict || item.guardVerdict || '');
  }

  function inferAction(item) {
    if (!item) return '';
    if (item.action) return item.action;
    const reason = String(item.reason || item.guardReason || '').toLowerCase();
    if (reason.includes('versiunea remote pare mai bună')) return 'REMOTE E MAI BUN';
    if (reason.includes('versiunea locală pare mai bună')) return 'AI DEJA VERSIUNEA MAI BUNĂ';
    const status = inferUserStatus(item);
    if (['AI DEJA', 'DESCĂRCAT DEJA', 'ACELAȘI CONȚINUT'].includes(status)) return 'NU DESCĂRCA';
    if (status === 'NU ÎL AI') return 'DESCARCĂ';
    if (status === 'NU S-A PUTUT VERIFICA') return 'REÎNCEARCĂ';
    return 'VERIFICĂ MANUAL';
  }

  function statusTone(status) {
    if (['AI DEJA', 'DESCĂRCAT DEJA', 'ACELAȘI CONȚINUT'].includes(status)) return 'goodText';
    if (status === 'NU ÎL AI') return 'dangerText';
    if (status === 'ALTĂ VERSIUNE' || status === 'PARE ACELAȘI' || status === 'POSIBIL DUPLICAT') return 'guardAmber';
    if (status === 'NU S-A PUTUT VERIFICA' || status === 'INDISPONIBIL' || status === 'LIMITĂ / COTĂ' || status === 'EROARE') return 'dangerText';
    return '';
  }

  function errorUserStatus(code) {
    const c = String(code || '').toUpperCase();
    if (c === 'MEGA_QUOTA' || c === 'MEGA_RATE_LIMIT') return 'LIMITĂ / COTĂ';
    if (['MEGA_BLOCKED', 'MEGA_AUTH', 'MEGA_KEY', 'MEGA_LINK', 'MEGA_NOT_FOUND'].includes(c)) return 'INDISPONIBIL';
    if (c === 'CANCELLED') return 'ANULAT';
    return 'EROARE';
  }

  function qualityText(q) {
    return ({ remote: 'remote pare mai bun', local: 'versiunea locală pare mai bună' })[q] || '';
  }

  function installV85Styles() {
    if (document.getElementById('v85GuardStyles')) return;
    const style = document.createElement('style');
    style.id = 'v85GuardStyles';
    style.textContent = `
      .guardAmber{color:#ffd979}
      .guardItem{border:1px solid #243140;border-radius:10px;padding:11px 12px;background:#0d141d;margin-bottom:8px}
      .guardStatus{font-size:13px;font-weight:850;letter-spacing:.02em}
      .guardAction{font-size:10px;font-weight:850;letter-spacing:.04em;border:1px solid #3a4d61;border-radius:999px;padding:4px 8px;background:#111b26;white-space:nowrap}
      .guardMeta{display:flex;gap:7px;flex-wrap:wrap;margin-top:6px}
      .guardMeta span{font-size:11px;color:#a9bbce;border:1px solid #2f4052;border-radius:999px;padding:3px 7px}
      .guardReason{margin-top:6px;line-height:1.45}
      .guardPath{display:block;margin-top:7px;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
      .guardLegendV85{display:flex;gap:7px;flex-wrap:wrap;margin-top:9px}
      .guardLegendV85 span{font-size:11px;border:1px solid #314155;border-radius:999px;padding:4px 7px;background:#0e151e}
      .errorStatusV85{display:inline-flex;margin:0 6px 4px 0;padding:3px 7px;border-radius:6px;font-size:10px;font-weight:850;letter-spacing:.03em;background:#4a2228;color:#ffadb5}
    `;
    document.head.appendChild(style);
  }

  function installModal() {
    if (document.getElementById('guardModal')) return;
    document.body.insertAdjacentHTML('beforeend', `
      <div class="smartModal hidden" id="guardModal" role="dialog" aria-modal="true" aria-labelledby="guardModalTitle">
        <div class="smartBox">
          <div class="smartHead">
            <div>
              <b id="guardModalTitle">🛡 Verdict final înainte de download</b>
              <div class="muted small" id="guardModeText">Verificare inteligentă a colecției locale</div>
            </div>
            <button class="btn" onclick="closeGuardReport()" aria-label="Închide">×</button>
          </div>
          <div class="smartBody">
            <div class="guardStats">
              <div class="guardStat"><span class="muted small">De descărcat</span><b class="goodText" id="guardDownloadCount">0</b></div>
              <div class="guardStat"><span class="muted small">Ai deja / blocate</span><b class="dangerText" id="guardDuplicateCount">0</b></div>
              <div class="guardStat"><span class="muted small">De verificat</span><b style="color:#ffd979" id="guardReviewCount">0</b></div>
            </div>
            <div class="noticeBlue" id="guardScanInfo">Se pregătește raportul…</div>
            <div class="guardLegendV85">
              <span>AI DEJA</span><span>DESCĂRCAT DEJA</span><span>ACELAȘI CONȚINUT</span><span>ALTĂ VERSIUNE</span><span>PARE ACELAȘI</span><span>NU ÎL AI</span><span>LIMITĂ / COTĂ</span><span>INDISPONIBIL</span>
            </div>
            <div class="guardList" id="guardDecisionList" style="margin-top:12px"></div>
          </div>
          <div class="modalFoot">
            <button class="btn" onclick="closeGuardReport()">Închide</button>
            <button class="btn warnbtn hidden" id="guardReviewOverride" onclick="downloadReviewOverride()">Descarcă oricum cele de verificat</button>
            <button class="btn primary" id="guardOpenQueue" onclick="closeGuardReport();goTab('downloads')">Deschide coada</button>
          </div>
        </div>
      </div>`);
  }

  function installControls() {
    const brandVersion = document.querySelector('.brand small');
    if (brandVersion) {
      api('/api/about').then(info => {
        brandVersion.textContent = `Professional • ${info.version}`;
      }).catch(() => {});
    }
    const megaHint = document.querySelector('#compare .section .sectionBody p.muted.small');
    if (megaHint) {
      megaHint.textContent = 'După scanare, sesiunea folderului MEGA rămâne pregătită temporar pentru preview și verificare; sesiunea anterioară este restaurată automat.';
    }

    const downloadButton = document.querySelector('button[onclick="downloadSelected()"]');
    if (downloadButton) {
      downloadButton.id = 'downloadGuardBtn';
      downloadButton.textContent = '🛡 Verifică inteligent + descarcă';
      downloadButton.title = 'Rescanează HDD-urile, verifică istoricul, hash-ul și variantele media înainte de orice download';
      const body = downloadButton.closest('.section')?.querySelector('.sectionBody');
      if (body && !document.getElementById('guardActivity')) {
        body.insertAdjacentHTML('afterbegin', '<div class="noticeBlue hidden" id="guardActivity" style="margin-bottom:12px"></div>');
      }
    }

    const method = document.getElementById('downloadMethod');
    const methodRow = method && method.closest('.row');
    if (methodRow && !document.getElementById('downloadGuardMode')) {
      methodRow.insertAdjacentHTML('afterend', `
        <div class="row" style="margin-top:12px">
          <div style="flex:1">
            <label>Protecție înainte de download</label>
            <select class="field" id="downloadGuardMode" style="margin-top:6px">
              <option value="smart">Smart Guard — recomandat</option>
              <option value="exact">Exact Guard — verificare integrală</option>
              <option value="ai">AI Guard — Smart + review media/AI</option>
            </select>
          </div>
          <div class="noticeBlue" style="flex:2;margin-top:20px">
            v8.5 verifică live HDD-urile, istoricul descărcărilor, hash/mărime și fișiere media recodate. Pentru video redenumit poate folosi durată + structură media + fingerprint pe 7 cadre.
          </div>
        </div>
        <label style="display:block;margin-top:10px"><input type="checkbox" id="liveRefreshCompare" checked/> Actualizează indexul live înainte de fiecare comparație</label>
        <div class="muted small" style="margin-top:4px">Un nume sau o mărime diferită nu mai înseamnă automat „NU ÎL AI”. Cazurile incerte sunt oprite pentru verificare, nu declarate duplicate fără dovadă.</div>`);
    }

    const fullVerify = document.getElementById('fullVerifyMaxMB');
    if (fullVerify) {
      const label = fullVerify.parentElement && fullVerify.parentElement.querySelector('label');
      if (label) label.textContent = 'Verificare integrală automată până la (MB)';
    }

    const downloadPanel = document.getElementById('downloads');
    const toolbar = downloadPanel && downloadPanel.querySelector('.toolbar');
    if (toolbar && !document.getElementById('pauseAllDownloads')) {
      toolbar.insertAdjacentHTML('afterbegin',
        '<button class="btn danger" id="stopAllDownloads" onclick="queueAction(\'stop-all\')">■ STOP TOT</button>' +
        '<button class="btn" id="pauseAllDownloads" onclick="queueAction(\'pause-all\')">Ⅱ Pauză TOT</button>');
    }
    const queueStats = downloadPanel && downloadPanel.querySelector('.queueStats');
    if (queueStats && !document.getElementById('megaProblemBanner')) {
      queueStats.insertAdjacentHTML('beforebegin', '<div class="warn hidden" id="megaProblemBanner" style="margin-bottom:12px"></div>');
    }

    const stats = downloadPanel && downloadPanel.querySelector('.queueStats');
    if (stats && !document.getElementById('qBlocked')) {
      stats.insertAdjacentHTML('beforeend', '<div class="qstat"><span class="muted small">Oprite ca duplicate</span><b id="qBlocked" class="dangerText">0</b></div>');
    }

    const help = document.getElementById('help-download');
    if (help) {
      const steps = help.querySelector('.helpSteps');
      if (steps) steps.innerHTML =
        '<div class="helpStep"><div>Selectează fișierele dorite în Rezultate.</div></div>' +
        '<div class="helpStep"><div>„🛡 Verifică inteligent + descarcă” rescanează live locațiile și verifică mai întâi dacă fișierul a fost deja descărcat.</div></div>' +
        '<div class="helpStep"><div>Pentru duplicate exacte folosește hash/bytes. Pentru poze și video modificate caută aceeași sursă prin fingerprint perceptual; video folosește 7 cadre și controlul duratei.</div></div>' +
        '<div class="helpStep"><div>Doar „NU ÎL AI” intră automat în coadă. „ALTĂ VERSIUNE”, „PARE ACELAȘI” și „POSIBIL DUPLICAT” cer verificare.</div></div>' +
        '<div class="helpStep"><div>În Descărcări ai Pauză TOT și STOP TOT; cota MEGA apare explicit ca „LIMITĂ / COTĂ”, iar linkurile/cheile indisponibile ca „INDISPONIBIL”.</div></div>';
    }
  }

  function syncModeFromConfig(attempt = 0) {
    const select = document.getElementById('downloadGuardMode');
    if (!select) return;
    if (typeof cfg !== 'undefined' && cfg && Object.keys(cfg).length) {
      select.value = cfg.downloadGuardMode || 'smart';
      const live = document.getElementById('liveRefreshCompare');
      if (live) live.checked = cfg.liveRefreshCompare !== false;
      return;
    }
    if (attempt < 20) setTimeout(() => syncModeFromConfig(attempt + 1), 100);
  }

  function decisionHTML(decision) {
    const status = inferUserStatus(decision);
    const action = inferAction(decision);
    const klass = statusTone(status);
    const local = decision.localPath ? `<code class="guardPath" title="${esc(decision.localPath)}">Local: ${esc(decision.localPath)}</code>` : '';
    const meta = [];
    if (Number(decision.similarity || 0) > 0) meta.push(`<span>similaritate ${Number(decision.similarity)}%</span>`);
    if (decision.qualityHint) meta.push(`<span>${esc(qualityText(decision.qualityHint))}</span>`);
    if (decision.exact) meta.push('<span>verificare exactă</span>');
    if (Number(decision.candidates || 0) > 0) meta.push(`<span>${Number(decision.candidates)} candidat(ți)</span>`);
    return `<div class="guardItem">
      <div class="row"><b class="guardStatus ${klass}">${esc(status)}</b><span class="guardAction right">${esc(action)}</span></div>
      <div style="margin-top:6px"><b>${esc(decision.name || 'fișier')}</b></div>
      ${meta.length ? `<div class="guardMeta">${meta.join('')}</div>` : ''}
      <div class="muted small guardReason">${esc(decision.reason || '')}</div>${local}
      <div class="muted small" style="margin-top:5px">Metodă: ${esc(decision.method || '—')}</div>
    </div>`;
  }

  function showGuardReport(report, request, added) {
    installModal();
    lastReport = report || { decisions: [], counts: {} };
    lastRequest = request || lastRequest;
    const counts = lastReport.counts || {};
    document.getElementById('guardDownloadCount').textContent = counts.DOWNLOAD || 0;
    document.getElementById('guardDuplicateCount').textContent = counts.DUPLICATE || 0;
    document.getElementById('guardReviewCount').textContent = counts.REVIEW || 0;
    document.getElementById('guardModeText').textContent = `${guardModeLabel(lastReport.mode)} • verdict final înainte de download`;
    const roots = (lastReport.scannedRoots || []).length;
    const duration = Number(lastReport.durationMs || 0) / 1000;
    document.getElementById('guardScanInfo').innerHTML =
      `<b>${Number(lastReport.scannedFiles || 0).toLocaleString('ro-RO')} fișiere locale</b> verificate live în ${roots} locație(i), în ${duration.toFixed(1)} secunde. ` +
      `<b>${Number(added || 0).toLocaleString('ro-RO')}</b> fișier(e) confirmate ca lipsă au intrat în coadă.`;
    const decisions = lastReport.decisions || [];
    const priority = d => d.verdict === 'REVIEW' ? 0 : d.verdict === 'DUPLICATE' ? 1 : 2;
    const visible = [...decisions].sort((a, b) => priority(a) - priority(b)).slice(0, 100);
    document.getElementById('guardDecisionList').innerHTML = visible.map(decisionHTML).join('') || '<div class="muted">Nu există decizii de afișat.</div>';
    if (decisions.length > visible.length) {
      document.getElementById('guardDecisionList').insertAdjacentHTML('beforeend', `<div class="muted small">…și încă ${decisions.length - visible.length} rezultate.</div>`);
    }
    const reviewButton = document.getElementById('guardReviewOverride');
    reviewButton.classList.toggle('hidden', !(counts.REVIEW > 0));
    document.getElementById('guardOpenQueue').classList.toggle('hidden', !(added > 0));
    document.getElementById('guardModal').classList.remove('hidden');
  }

  window.closeGuardReport = function () {
    const modal = document.getElementById('guardModal');
    if (modal) modal.classList.add('hidden');
  };

  window.downloadReviewOverride = async function () {
    const ids = (lastReport?.decisions || []).filter(x => x.verdict === 'REVIEW').map(x => x.resultId);
    if (!ids.length || !lastRequest) return;
    if (!confirm(`Aceste ${ids.length} fișiere NU sunt confirmate ca lipsă. Le descarci totuși? Duplicatele certe și cele deja descărcate rămân blocate.`)) return;
    const button = document.getElementById('guardReviewOverride');
    button.disabled = true;
    button.textContent = 'Se reverifică…';
    try {
      const request = { ...lastRequest, ids, allowReview: true };
      const data = await api('/api/queue/add', {
        method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(request)
      });
      await loadResults();
      showGuardReport(data.guard, request, data.added);
      toast(`${data.added || 0} fișiere neconfirmate adăugate explicit`);
    } catch (error) {
      toast(error.message);
    } finally {
      button.disabled = false;
      button.textContent = 'Descarcă oricum cele de verificat';
    }
  };

  function installFunctionOverrides() {
    const originalSaveRules = saveRules;
    saveRules = async function () {
      const mode = document.getElementById('downloadGuardMode');
      if (mode) cfg.downloadGuardMode = mode.value || 'smart';
      const live = document.getElementById('liveRefreshCompare');
      if (live) cfg.liveRefreshCompare = live.checked;
      return originalSaveRules();
    };

    downloadSelected = async function () {
      const ids = idsForAction();
      if (!ids.length) return toast('Selectează fișiere');
      const destination = cfg.downloadDir || document.getElementById('downloadDir')?.value || '';
      if (!destination) return toast('Setează folderul de download');
      const mode = document.getElementById('downloadGuardMode')?.value || cfg.downloadGuardMode || 'smart';
      const request = { ids, engine: cfg.downloadMethod || 'auto', destination, guardMode: mode };
      const button = document.getElementById('downloadGuardBtn');
      if (button) {
        button.disabled = true;
        button.textContent = '🛡 Verific HDD + istoric + media…';
      }
      const started = Date.now();
      showActivity(`Verificarea a început pentru ${ids.length} fișier(e): index live, istoric, hash și candidați media…`);
      clearInterval(guardTicker);
      guardTicker = setInterval(() => {
        const seconds = Math.floor((Date.now() - started) / 1000);
        showActivity(`Verificare în curs: ${seconds}s • ${ids.length} fișier(e). Cazurile media dificile pot necesita ffprobe/fingerprint.`);
      }, 1000);
      toast('Smart Guard verifică dacă fișierele chiar lipsesc…');
      try {
        const data = await api('/api/queue/add', {
          method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(request)
        });
        await loadResults();
        showGuardReport(data.guard, request, data.added);
        showActivity(data.message || `${data.added || 0} fișier(e) confirmate ca lipsă au intrat în coadă.`, data.added > 0 ? 'ok' : 'info');
        await loadQueue();
      } catch (error) {
        showActivity(`Download oprit cu eroare: ${error.message}`, 'error');
        toast(error.message);
      } finally {
        clearInterval(guardTicker);
        guardTicker = null;
        if (button) {
          button.disabled = false;
          button.textContent = '🛡 Verifică inteligent + descarcă';
        }
      }
    };

    const originalJD2 = sendSelectedJD2;
    sendSelectedJD2 = async function () {
      const ids = idsForAction();
      if (!ids.length) return toast('Selectează fișiere');
      try {
        const data = await api('/api/download/jd2', {
          method: 'POST', headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ ids, folder: cfg.jdFolder || '' })
        });
        await loadResults();
        showGuardReport(data.guard, { ids, destination: cfg.downloadDir || '', guardMode: cfg.downloadGuardMode || 'smart' }, data.count);
        toast(`JDownloader: ${data.count} link(uri) confirmate ca lipsă`);
      } catch (error) {
        if (!String(error.message).includes('404')) return toast(error.message);
        return originalJD2();
      }
    };

    qStatusLabel = function (status) {
      return ({ queued: 'ÎN COADĂ', running: 'DESCARCĂ', paused: 'PAUZĂ', completed: 'GATA', failed: 'EROARE', cancelled: 'ANULAT', blocked: 'NU DESCĂRCA' })[status] || status;
    };

    loadQueue = async function () {
      try {
        const data = await api('/api/queue/list');
        const counts = data.summary.counts || {};
        setText('qQueued', counts.queued || 0);
        setText('qRunning', counts.running || 0);
        setText('qPaused', counts.paused || 0);
        setText('qCompleted', counts.completed || 0);
        setText('qFailed', counts.failed || 0);
        setText('qBlocked', counts.blocked || 0);
        setText('qBytes', `${data.summary.bytesDoneText} / ${data.summary.bytesTotalText}`);
        setText('qFolder', data.downloadDir || '');
        const megaBanner = document.getElementById('megaProblemBanner');
        if (megaBanner) {
          const problem = data.megaStatus;
          megaBanner.classList.toggle('hidden', !problem);
          megaBanner.innerHTML = problem ? `<span class="errorStatusV85">${esc(errorUserStatus(problem.code))}</span><b>${esc(problem.title || 'MEGA')}</b> <span class="sourcePill">${esc(problem.code || '')}</span><br>${esc(problem.message || '')}<br><b>Ce faci:</b> ${esc(problem.action || '')}` : '';
        }
        const body = document.getElementById('queueBody');
        body.innerHTML = data.jobs.map(job => {
          const percent = job.bytesTotal > 0 ? Math.min(100, job.bytesDone / job.bytesTotal * 100) : 0;
          const checked = queueSelected.has(job.id) ? 'checked' : '';
          const guardStatus = job.guardVerdict ? inferUserStatus({ guardVerdict: job.guardVerdict, guardMethod: job.guardMethod }) : '';
          const guard = guardStatus ? `<div class="muted small">🛡 ${esc(guardStatus)}${job.guardMethod ? ` • ${esc(job.guardMethod)}` : ''}</div>` : '';
          const stage = job.stage ? `<div class="muted small">Etapă: ${esc(job.stage)}</div>` : '';
          const errorStatus = job.error ? `<span class="errorStatusV85">${esc(errorUserStatus(job.errorCode))}</span>` : '';
          const destination = job.error ? `<span class="dangerText">${errorStatus}<b>${esc(job.errorTitle || 'Eroare')}</b>${job.errorCode ? ` [${esc(job.errorCode)}]` : ''}<br>${esc(job.error)}${job.errorAction ? `<br><b>Ce faci:</b> ${esc(job.errorAction)}` : ''}</span>${stage}` : `${esc(job.outputPath || job.destination || '')}${stage}`;
          return `<tr><td><input class="check qcheck" type="checkbox" data-qid="${esc(job.id)}" ${checked} onchange="queueToggle('${esc(job.id)}',this.checked)"/></td><td><b class="${qClass(job.status)}">${qStatusLabel(job.status)}</b><div class="muted small">P${job.priority || 0}</div></td><td><b>${esc(job.name)}</b><div class="muted small">${esc(job.source || '')}</div>${guard}</td><td>${esc(job.engine || 'auto')}${job.gid ? `<div class="muted small">GID ${esc(job.gid)}</div>` : ''}</td><td><div class="downloadBar"><i style="width:${percent}%"></i></div><div class="muted small">${fmt(job.bytesDone || 0)} / ${job.bytesTotal > 0 ? fmt(job.bytesTotal) : '?'}</div></td><td>${job.speedBps > 0 ? fmt(job.speedBps) + '/s' : '—'}</td><td>${qEta(job.etaSeconds)}</td><td>${job.attempts || 0}/${job.maxRetries || 0}</td><td class="path">${destination}</td></tr>`;
        }).join('') || '<tr><td colspan="9" class="muted" style="padding:25px;text-align:center">Coada este goală. Selectează rezultate și apasă „🛡 Verifică inteligent + descarcă”.</td></tr>';
      } catch (error) {
        const megaBanner = document.getElementById('megaProblemBanner');
        if (megaBanner) {
          megaBanner.classList.remove('hidden');
          megaBanner.textContent = 'Nu pot citi starea cozii: ' + error.message;
        }
      }
    };

    const originalQueueAction = queueAction;
    queueAction = async function (action) {
      if (action !== 'pause-all' && action !== 'stop-all') return originalQueueAction(action);
      if (action === 'stop-all' && !confirm('Oprești și anulezi TOATE descărcările active, în coadă și puse pe pauză?')) return;
      try {
        await api('/api/queue/action', {
          method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ ids: [], action })
        });
        queueSelected.clear();
        await loadQueue();
        toast(action === 'pause-all' ? 'Toate descărcările au fost puse pe pauză' : 'Toate descărcările au fost oprite');
      } catch (error) {
        toast(error.message);
      }
    };

    const originalShowDetail = showDetail;
    showDetail = async function (row) {
      await originalShowDetail(row);
      if (!row.guardVerdict) return;
      const detail = document.getElementById('detail');
      if (!detail) return;
      const status = inferUserStatus(row);
      const action = inferAction(row);
      const klass = row.guardVerdict === 'DUPLICATE' ? 'VERIFIED' : row.guardVerdict === 'DOWNLOAD' ? 'MISSING' : 'POSSIBLE';
      const extra = row.visualScore ? ` • similaritate ${esc(String(row.visualScore))}%` : '';
      const value = `<span class="badge ${klass}">${esc(status)}</span> <b style="margin-left:6px">${esc(action)}</b> ` +
        `<span class="muted small">${esc(row.guardMethod || '')}${extra}${row.guardReason ? ' • ' + esc(row.guardReason) : ''}</span>`;
      detail.insertAdjacentHTML('beforeend', `<b>Smart Guard</b><span>${value}</span>`);
    };
  }

  document.addEventListener('DOMContentLoaded', () => {
    installV85Styles();
    installModal();
    installControls();
    installFunctionOverrides();
    syncModeFromConfig();
  });
})();

// v8.5 MEGA player prewarm: selecting a MEGA video prepares WebDAV before the
// user presses the external-player button. The existing backend fast path then
// reuses the exact same stream URL instead of paying setup latency on click.
(() => {
  'use strict';
  let prewarmTimer = null;
  let lastRequestedId = 0;
  let generation = 0;

  function isMegaVideoRow(row) {
    const remote = row && row.remote;
    if (!remote || String(remote.source || '').toUpperCase() !== 'MEGA') return false;
    const name = String(remote.name || remote.path || '').toLowerCase();
    return /\.(mp4|webm|ogv|mov|m4v|mkv|avi|flv|ts|mts|m2ts)$/.test(name);
  }

  function scheduleMegaPrewarm(row) {
    clearTimeout(prewarmTimer);
    const id = Number(row && row.id || 0);
    if (!id || !isMegaVideoRow(row)) return;
    const myGeneration = ++generation;
    prewarmTimer = setTimeout(async () => {
      if (myGeneration !== generation || id === lastRequestedId) return;
      lastRequestedId = id;
      try {
        await api('/api/remote-preview/start', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ id })
        });
      } catch (_) {
        // Prewarm is opportunistic. The normal player action remains the source
        // of user-visible errors and can retry with the backend's full messages.
        if (lastRequestedId === id) lastRequestedId = 0;
      }
    }, 300);
  }

  document.addEventListener('DOMContentLoaded', () => {
    if (typeof showDetail !== 'function') return;
    const previousShowDetail = showDetail;
    showDetail = async function (row) {
      await previousShowDetail(row);
      scheduleMegaPrewarm(row);
    };
  });
})();
