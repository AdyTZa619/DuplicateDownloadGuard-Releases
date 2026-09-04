(() => {
  'use strict';

  const escQ = v => String(v ?? '').replace(/[&<>"']/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));

  function installButton() {
    if (document.getElementById('qualityIntelBtnV860')) return;
    const anchor = document.getElementById('mediaAnalyzeBtn');
    const parent = anchor?.parentElement;
    if (!parent) return;
    const btn = document.createElement('button');
    btn.className = 'btn';
    btn.id = 'qualityIntelBtnV860';
    btn.textContent = '★ Calitate inteligentă';
    btn.title = 'Compară explicabil rezoluția, completitudinea, bit depth/HDR, FPS, audio și pistele. Nu schimbă verdictul de duplicat.';
    btn.onclick = () => window.runQualityIntelligenceV860();
    parent.insertBefore(btn, anchor.nextSibling);
  }

  function streamSummary(s) {
    if (!s) return '—';
    const parts = [];
    if (s.width && s.height) parts.push(`${s.width}×${s.height}`);
    if (s.codec) parts.push(s.codec.toUpperCase());
    if (s.bitDepth) parts.push(`${s.bitDepth}-bit`);
    if (s.hdr) parts.push('HDR');
    if (s.fps) parts.push(`${Number(s.fps).toFixed(2)} fps`);
    return parts.join(' • ') || '—';
  }

  function infoSummary(i) {
    if (!i?.ok) return `<span class="dangerText">${escQ(i?.error || 'metadate indisponibile')}</span>`;
    const audio = Array.isArray(i.audio) ? i.audio : [];
    const maxCh = audio.reduce((m, a) => Math.max(m, Number(a.channels || 0)), 0);
    const bits = [streamSummary(i.video)];
    if (i.duration) bits.push(`${Number(i.duration).toFixed(1)} sec`);
    if (i.audioTracks) bits.push(`${i.audioTracks} audio${maxCh ? ` • max ${maxCh}ch` : ''}`);
    if (i.subtitleTracks) bits.push(`${i.subtitleTracks} subtitrări`);
    return bits.filter(Boolean).join(' • ');
  }

  function renderQuality(d) {
    const box = document.getElementById('mediaInfo');
    const score = document.getElementById('mediaScore');
    if (!box) return;
    const dec = d?.decision || {};
    const factors = Array.isArray(dec.factors) ? dec.factors : [];
    const tone = dec.verdict === 'remote-better' ? '#8ee6bd' : dec.verdict === 'local-better' ? '#8fc9ff' : dec.verdict === 'incomplete' ? '#ff9aa5' : '#ffd979';
    if (score) score.textContent = `${dec.remoteScore || 0} ↔ ${dec.localScore || 0}`;
    box.innerHTML = `
      <div style="border:1px solid #314155;border-radius:10px;padding:12px;background:#0d141d">
        <div style="display:flex;justify-content:space-between;gap:12px;align-items:center;flex-wrap:wrap">
          <b style="color:${tone};font-size:14px">${escQ(dec.userVerdict || 'CALITATE NECUNOSCUTĂ')}</b>
          <span class="sourcePill">încredere ${escQ(dec.confidence || '?')}</span>
        </div>
        ${dec.caution ? `<div class="warn" style="margin-top:10px">${escQ(dec.caution)}</div>` : ''}
        <div class="two" style="margin-top:12px">
          <div class="metaBox"><b>REMOTE</b><div class="muted small" style="margin-top:7px;line-height:1.55">${infoSummary(d.remote)}</div></div>
          <div class="metaBox"><b>LOCAL</b><div class="muted small" style="margin-top:7px;line-height:1.55">${infoSummary(d.local)}</div></div>
        </div>
        <div style="margin-top:12px;display:flex;flex-direction:column;gap:7px">
          ${factors.map(f => {
            const icon = f.winner === 'remote' ? '→ REMOTE' : f.winner === 'local' ? '→ LOCAL' : f.winner === 'warning' ? '⚠' : '=';
            return `<div class="guardItem" style="margin:0"><div style="display:flex;justify-content:space-between;gap:10px"><b>${escQ(f.factor)}</b><span class="muted small">${escQ(icon)}${f.weight ? ` • pondere ${f.weight}` : ''}</span></div><div class="guardMeta"><span>R: ${escQ(f.remote || '—')}</span><span>L: ${escQ(f.local || '—')}</span></div><div class="guardReason">${escQ(f.reason || '')}</div></div>`;
          }).join('') || '<div class="muted">Nu există suficiente criterii comparabile.</div>'}
        </div>
        <div class="muted small" style="margin-top:10px">Recomandarea de calitate nu este folosită ca dovadă că două fișiere sunt duplicate. Identitatea rămâne responsabilitatea Smart Media Guard.</div>
      </div>`;
  }

  window.runQualityIntelligenceV860 = async function() {
    if (!currentRow?.id) return toast('Selectează un rezultat');
    const btn = document.getElementById('qualityIntelBtnV860');
    if (btn) { btn.disabled = true; btn.textContent = '★ Analizez…'; }
    const box = document.getElementById('mediaInfo');
    if (box) box.innerHTML = '<div class="previewLoading"><div class="spin"></div><b>Analizez calitatea REMOTE ↔ LOCAL…</b><span class="small">ffprobe: video, HDR, bit depth, FPS, audio și completitudine.</span></div>';
    try {
      const d = await api(`/api/media/quality?id=${encodeURIComponent(currentRow.id)}`);
      renderQuality(d);
    } catch (err) {
      if (box) box.innerHTML = `<span class="dangerText">${escQ(err.message || err)}</span>`;
      toast(err.message || String(err));
    } finally {
      if (btn) { btn.disabled = false; btn.textContent = '★ Calitate inteligentă'; }
    }
  };

  function boot() { installButton(); }
  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', boot, {once:true});
  else boot();
})();