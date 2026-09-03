// ExactGuard v8.3 UI layer. It keeps the existing interface intact and adds a
// mandatory, visible pre-download safety report for every download route.
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

  const verdictLabel = verdict => ({
    DOWNLOAD: 'DESCĂRCARE SIGURĂ',
    DUPLICATE: 'DUPLICAT BLOCAT',
    REVIEW: 'NECESITĂ REVIEW'
  })[verdict] || verdict;

  function installModal() {
    if (document.getElementById('guardModal')) return;
    document.body.insertAdjacentHTML('beforeend', `
      <div class="smartModal hidden" id="guardModal" role="dialog" aria-modal="true" aria-labelledby="guardModalTitle">
        <div class="smartBox">
          <div class="smartHead">
            <div>
              <b id="guardModalTitle">🛡 Raport Download Guard</b>
              <div class="muted small" id="guardModeText">Verificare finală înainte de download</div>
            </div>
            <button class="btn" onclick="closeGuardReport()" aria-label="Închide">×</button>
          </div>
          <div class="smartBody">
            <div class="guardStats">
              <div class="guardStat"><span class="muted small">Pot fi descărcate</span><b class="goodText" id="guardDownloadCount">0</b></div>
              <div class="guardStat"><span class="muted small">Duplicate blocate</span><b class="dangerText" id="guardDuplicateCount">0</b></div>
              <div class="guardStat"><span class="muted small">Necesită review</span><b style="color:#ffd979" id="guardReviewCount">0</b></div>
            </div>
            <div class="noticeBlue" id="guardScanInfo">Se pregătește raportul…</div>
            <div class="guardList" id="guardDecisionList" style="margin-top:12px"></div>
          </div>
          <div class="modalFoot">
            <button class="btn" onclick="closeGuardReport()">Închide</button>
            <button class="btn warnbtn hidden" id="guardReviewOverride" onclick="downloadReviewOverride()">Descarcă și REVIEW</button>
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
      megaHint.textContent = 'După scanare, sesiunea folderului MEGA rămâne pregătită temporar pentru preview și player; sesiunea anterioară este restaurată la oprirea streamului sau după expirare.';
    }

    const downloadButton = document.querySelector('button[onclick="downloadSelected()"]');
    if (downloadButton) {
      downloadButton.id = 'downloadGuardBtn';
      downloadButton.textContent = '🛡 Verifică + descarcă';
      downloadButton.title = 'Scanează live HDD-urile, confirmă duplicatele și descarcă numai lipsurile sigure';
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
            Scanează live toate locațiile indexate. Numele diferit nu mai înseamnă automat „lipsește”. AI poate cere review, dar nu poate declara singur un duplicat exact.
          </div>
        </div>
        <label style="display:block;margin-top:10px"><input type="checkbox" id="liveRefreshCompare" checked/> Actualizează indexul live înainte de fiecare comparație</label>
        <div class="muted small" style="margin-top:4px">Fișierele copiate, mutate sau redenumite după ultima indexare intră în verdictul inițial; candidații de aceeași mărime devin REVIEW/POSIBIL, nu LIPSEȘTE.</div>`);
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
      stats.insertAdjacentHTML('beforeend', '<div class="qstat"><span class="muted small">Duplicate blocate</span><b id="qBlocked" class="dangerText">0</b></div>');
    }

    const help = document.getElementById('help-download');
    if (help) {
      const steps = help.querySelector('.helpSteps');
      if (steps) steps.innerHTML =
        '<div class="helpStep"><div>Selectează fișierele dorite în Rezultate.</div></div>' +
        '<div class="helpStep"><div>„🛡 Verifică + descarcă” rescanează live HDD-urile și verifică toți candidații de aceeași mărime, indiferent de nume.</div></div>' +
        '<div class="helpStep"><div>Numai „DESCĂRCARE SIGURĂ” intră automat în coadă. Duplicatele exacte sunt blocate, iar cazurile ambigue rămân REVIEW.</div></div>' +
        '<div class="helpStep"><div>În Descărcări ai Pauză TOT și STOP TOT, fără ferestre CMD care să preia controlul ecranului.</div></div>';
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
    const klass = decision.verdict === 'DUPLICATE' ? 'dangerText' : decision.verdict === 'DOWNLOAD' ? 'goodText' : '';
    const local = decision.localPath ? `<code title="${esc(decision.localPath)}">Local: ${esc(decision.localPath)}</code>` : '';
    return `<div class="guardItem">
      <div class="row"><b class="${klass}">${verdictLabel(decision.verdict)}</b><span class="right sourcePill">${esc(decision.method || '—')}</span></div>
      <div style="margin-top:5px"><b>${esc(decision.name || 'fișier')}</b></div>
      <div class="muted small" style="margin-top:4px">${esc(decision.reason || '')}</div>${local}
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
    document.getElementById('guardModeText').textContent = `${guardModeLabel(lastReport.mode)} • verificare finală înainte de download`;
    const roots = (lastReport.scannedRoots || []).length;
    const duration = Number(lastReport.durationMs || 0) / 1000;
    document.getElementById('guardScanInfo').innerHTML =
      `<b>${Number(lastReport.scannedFiles || 0).toLocaleString('ro-RO')} fișiere locale</b> verificate live în ${roots} locație(i), în ${duration.toFixed(1)} secunde. ` +
      `<b>${Number(added || 0).toLocaleString('ro-RO')}</b> job(uri) sigure au fost adăugate în coadă.`;
    const decisions = lastReport.decisions || [];
    const visible = decisions.filter(x => x.verdict !== 'DOWNLOAD').concat(decisions.filter(x => x.verdict === 'DOWNLOAD')).slice(0, 100);
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
    if (!confirm(`Aceste ${ids.length} fișiere NU sunt confirmate ca lipsă. Le descarci totuși? Duplicatele exacte rămân blocate.`)) return;
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
      toast(`${data.added || 0} joburi REVIEW adăugate explicit`);
    } catch (error) {
      toast(error.message);
    } finally {
      button.disabled = false;
      button.textContent = 'Descarcă și REVIEW';
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
        button.textContent = '🛡 Scanez HDD + verific…';
      }
      const started = Date.now();
      showActivity(`Verificarea a început pentru ${ids.length} fișier(e). Scanez locațiile locale și verific toate deciziile…`);
      clearInterval(guardTicker);
      guardTicker = setInterval(() => {
        const seconds = Math.floor((Date.now() - started) / 1000);
        showActivity(`Verificare în curs: ${seconds}s • ${ids.length} fișier(e). Nu închide aplicația.`);
      }, 1000);
      toast('Download Guard scanează live toate locațiile indexate…');
      try {
        const data = await api('/api/queue/add', {
          method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(request)
        });
        await loadResults();
        showGuardReport(data.guard, request, data.added);
        showActivity(data.message || `${data.added || 0} job(uri) adăugate în coadă.`, data.added > 0 ? 'ok' : 'info');
        await loadQueue();
      } catch (error) {
        showActivity(`Download oprit cu eroare: ${error.message}`, 'error');
        toast(error.message);
      } finally {
        clearInterval(guardTicker);
        guardTicker = null;
        if (button) {
          button.disabled = false;
          button.textContent = '🛡 Verifică + descarcă';
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
        toast(`JDownloader: ${data.count} link(uri) sigure`);
      } catch (error) {
        // Keep the legacy function available as a compatibility fallback only
        // if an older backend does not yet expose the guarded response.
        if (!String(error.message).includes('404')) return toast(error.message);
        return originalJD2();
      }
    };

    qStatusLabel = function (status) {
      return ({ queued: 'ÎN COADĂ', running: 'DESCARCĂ', paused: 'PAUZĂ', completed: 'GATA', failed: 'EROARE', cancelled: 'ANULAT', blocked: 'BLOCAT DUPLICAT' })[status] || status;
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
          megaBanner.innerHTML = problem ? `<b>${esc(problem.title || 'MEGA')}</b> <span class="sourcePill">${esc(problem.code || '')}</span><br>${esc(problem.message || '')}<br><b>Ce faci:</b> ${esc(problem.action || '')}` : '';
        }
        const body = document.getElementById('queueBody');
        body.innerHTML = data.jobs.map(job => {
          const percent = job.bytesTotal > 0 ? Math.min(100, job.bytesDone / job.bytesTotal * 100) : 0;
          const checked = queueSelected.has(job.id) ? 'checked' : '';
          const guard = job.guardVerdict ? `<div class="muted small">🛡 ${esc(job.guardVerdict)} • ${esc(job.guardMethod || '')}</div>` : '';
          const stage = job.stage ? `<div class="muted small">Etapă: ${esc(job.stage)}</div>` : '';
          const destination = job.error ? `<span class="dangerText"><b>${esc(job.errorTitle || 'Eroare')}</b>${job.errorCode ? ` [${esc(job.errorCode)}]` : ''}<br>${esc(job.error)}${job.errorAction ? `<br><b>Ce faci:</b> ${esc(job.errorAction)}` : ''}</span>${stage}` : `${esc(job.outputPath || job.destination || '')}${stage}`;
          return `<tr><td><input class="check qcheck" type="checkbox" data-qid="${esc(job.id)}" ${checked} onchange="queueToggle('${esc(job.id)}',this.checked)"/></td><td><b class="${qClass(job.status)}">${qStatusLabel(job.status)}</b><div class="muted small">P${job.priority || 0}</div></td><td><b>${esc(job.name)}</b><div class="muted small">${esc(job.source || '')}</div>${guard}</td><td>${esc(job.engine || 'auto')}${job.gid ? `<div class="muted small">GID ${esc(job.gid)}</div>` : ''}</td><td><div class="downloadBar"><i style="width:${percent}%"></i></div><div class="muted small">${fmt(job.bytesDone || 0)} / ${job.bytesTotal > 0 ? fmt(job.bytesTotal) : '?'}</div></td><td>${job.speedBps > 0 ? fmt(job.speedBps) + '/s' : '—'}</td><td>${qEta(job.etaSeconds)}</td><td>${job.attempts || 0}/${job.maxRetries || 0}</td><td class="path">${destination}</td></tr>`;
        }).join('') || '<tr><td colspan="9" class="muted" style="padding:25px;text-align:center">Coada este goală. Selectează rezultate și apasă „🛡 Verifică + descarcă”.</td></tr>';
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
      const value = `<span class="badge ${row.guardVerdict === 'DUPLICATE' ? 'BLOCKED' : row.guardVerdict === 'DOWNLOAD' ? 'VERIFIED' : 'POSSIBLE'}">${verdictLabel(row.guardVerdict)}</span> ` +
        `<span class="muted small">${esc(row.guardMethod || '')}${row.guardReason ? ' • ' + esc(row.guardReason) : ''}</span>`;
      detail.insertAdjacentHTML('beforeend', `<b>Download Guard</b><span>${value}</span>`);
    };
  }

  document.addEventListener('DOMContentLoaded', () => {
    installModal();
    installControls();
    installFunctionOverrides();
    syncModeFromConfig();
  });
})();
