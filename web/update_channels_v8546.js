(()=>{
  const TEST_CHANNEL_LABEL='TEST';
  const TOP_ID='ddgUpdateCorner';
  const TEST_BOX_ID='ddgTestUpdaterBox';
  let lastStable=null,lastTest=null,checking=false;

  function ensureStyles(){
    if(document.getElementById('ddgUpdateCornerStyle'))return;
    const s=document.createElement('style');
    s.id='ddgUpdateCornerStyle';
    s.textContent=`
      #${TOP_ID}{display:flex;align-items:center;gap:7px;margin-right:10px}
      #${TOP_ID} .ddgUpdateBtn{border:1px solid #3c5066;background:#172131;color:#eaf2ff;padding:7px 10px;border-radius:8px;cursor:pointer;font-weight:700}
      #${TOP_ID} .ddgUpdateBtn:hover{border-color:#4da3ff}
      #${TOP_ID} .ddgUpdateBtn.stable{border-color:#2e7d61;color:#8ff0c8;background:#10281f}
      #${TOP_ID} .ddgUpdateBtn.test{border-color:#725aa8;color:#cebaff;background:#211a32}
      #${TOP_ID} .ddgUpdateBtn.busy{opacity:.65;cursor:wait}
      .ddgTestUpdater{margin-top:14px;padding:13px;border:1px solid #4d3f72;background:#171326;border-radius:10px}
      .ddgTestUpdater .ddgTestHead{display:flex;align-items:center;gap:8px;margin-bottom:8px}
      .ddgTestUpdater .ddgTestBadge{font-size:10px;font-weight:800;letter-spacing:.08em;padding:3px 6px;border-radius:999px;background:#2e2450;color:#d7c8ff;border:1px solid #6b55a0}
      .ddgTestUpdater .ddgTestActions{display:flex;gap:8px;flex-wrap:wrap;margin-top:10px}
    `;
    document.head.appendChild(s);
  }

  function ensureCorner(){
    ensureStyles();
    let box=document.getElementById(TOP_ID);
    if(box)return box;
    const topStatus=document.getElementById('topStatus');
    if(!topStatus||!topStatus.parentElement)return null;
    box=document.createElement('span');
    box.id=TOP_ID;
    topStatus.parentElement.insertBefore(box,topStatus.parentElement.firstChild);
    return box;
  }

  function ensureTestBox(){
    if(document.getElementById(TEST_BOX_ID))return;
    const updateText=document.getElementById('updateStatusText');
    if(!updateText)return;
    const host=updateText.parentElement;
    if(!host)return;
    const box=document.createElement('div');
    box.id=TEST_BOX_ID;
    box.className='ddgTestUpdater';
    box.innerHTML=`<div class="ddgTestHead"><span class="ddgTestBadge">${TEST_CHANNEL_LABEL}</span><b>Canal separat pentru versiuni de test</b></div>
      <div class="muted small">Primește build-uri de probă fără să schimbe canalul Stable. Folosește-l doar când vrei să verifici un fix înainte să fie publicat tuturor.</div>
      <div class="ddgTestActions"><button class="btn" id="ddgCheckTestUpdate">↻ Verifică TEST</button><button class="btn" id="ddgInstallTestUpdate">⬇ Instalează TEST</button></div>
      <div class="muted small" id="ddgTestUpdateStatus" style="margin-top:8px">Nu a fost verificat încă.</div>`;
    host.appendChild(box);
    document.getElementById('ddgCheckTestUpdate').onclick=()=>checkChannel('test',false);
    document.getElementById('ddgInstallTestUpdate').onclick=()=>installChannel('test');
  }

  function channelName(ch){return ch==='test'?'TEST':'Stable'}
  async function checkChannel(channel,silent=true){
    try{
      const d=await api('/api/update/check?channel='+encodeURIComponent(channel));
      if(channel==='test')lastTest=d;else lastStable=d;
      if(channel==='test'){
        const el=document.getElementById('ddgTestUpdateStatus');
        if(el)el.textContent=!d.configured?(d.message||'Canal TEST neconfigurat'):d.newer?`TEST disponibil: ${d.manifest.version} — ${d.manifest.notes||''}`:`Niciun TEST mai nou • ${d.manifest.version}`;
      }else{
        const el=document.getElementById('updateStatusText');
        if(el&&d.configured)el.textContent=d.newer?`Update Stable disponibil: ${d.manifest.version} — ${d.manifest.notes||''}`:`Ești la zi • ${d.manifest.version}`;
      }
      renderCorner();
      if(!silent&&d.newer&&typeof toast==='function')toast(`${channelName(channel)} disponibil: ${d.manifest.version}`);
      return d;
    }catch(e){
      if(channel==='test'){
        const el=document.getElementById('ddgTestUpdateStatus');
        if(el)el.textContent='Canal TEST indisponibil momentan: '+e.message;
      }
      return null;
    }
  }

  function renderCorner(){
    const box=ensureCorner();
    if(!box)return;
    const buttons=[];
    if(lastStable?.newer){
      buttons.push(`<button class="ddgUpdateBtn stable" data-ch="stable" title="Instalează update-ul Stable">↑ Update ${lastStable.manifest.version}</button>`);
    }
    if(lastTest?.newer){
      buttons.push(`<button class="ddgUpdateBtn test" data-ch="test" title="Instalează versiunea de test">🧪 TEST ${lastTest.manifest.version}</button>`);
    }
    box.innerHTML=buttons.join('');
    box.querySelectorAll('button[data-ch]').forEach(b=>b.onclick=()=>installChannel(b.dataset.ch));
  }

  async function installChannel(channel){
    const state=channel==='test'?lastTest:lastStable;
    const version=state?.manifest?.version||channelName(channel);
    const warning=channel==='test'?'Aceasta este o versiune de TEST. Are backup + health-check + rollback automat, dar nu este încă Stable.\n\n':'';
    if(!confirm(`${warning}Instalez ${version} acum? Aplicația se va închide și reporni automat.`))return;
    const box=ensureCorner();
    const btn=box?.querySelector(`[data-ch="${channel}"]`);
    if(btn){btn.classList.add('busy');btn.disabled=true;btn.textContent='Se instalează…'}
    try{
      const d=await api('/api/update/install-online?channel='+encodeURIComponent(channel),{method:'POST'});
      if(!d.newer){
        if(typeof toast==='function')toast('Nu există o versiune mai nouă pe acest canal');
        await checkChannel(channel,true);
      }
    }catch(e){
      if(typeof toast==='function')toast('Update: '+e.message);
      if(btn){btn.classList.remove('busy');btn.disabled=false}
    }
  }

  async function pollUpdates(){
    if(checking)return;
    checking=true;
    try{
      await Promise.all([checkChannel('stable',true),checkChannel('test',true)]);
    }finally{checking=false}
  }

  function boot(){
    ensureCorner();
    ensureTestBox();
    setTimeout(pollUpdates,1600);
    setInterval(pollUpdates,10*60*1000);
  }

  if(document.readyState==='loading')document.addEventListener('DOMContentLoaded',boot);else boot();
  window.ddgCheckUpdateChannels=pollUpdates;
})();
