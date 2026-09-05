(() => {
  'use strict';

  const sleep = ms => new Promise(resolve => setTimeout(resolve, ms));

  async function getJSONWithReconnect(url, attempts = 3) {
    let lastError = null;
    for (let i = 0; i < attempts; i++) {
      try {
        const sep = url.includes('?') ? '&' : '?';
        const response = await fetch(`${url}${sep}_ddg=${Date.now()}_${i}`, {
          method: 'GET',
          cache: 'no-store',
          headers: {'Cache-Control': 'no-cache'}
        });
        if (!response.ok) throw new Error((await response.text()).trim() || `HTTP ${response.status}`);
        return await response.json();
      } catch (err) {
        lastError = err;
        if (i + 1 < attempts) await sleep(250 * (i + 1));
      }
    }
    throw lastError || new Error('Conexiunea locală cu updaterul nu răspunde');
  }

  function setUpdateText(text) {
    const el = document.getElementById('updateStatusText');
    if (el) el.textContent = text;
  }

  function installOverrides() {
    // The original UI functions are declared by index.html. Override only the
    // read/check path: update installation semantics remain untouched.
    window.loadUpdateStatus = async function () {
      try {
        const d = await getJSONWithReconnect('/api/update/status', 3);
        setUpdateText(`Versiune instalată: ${d.version} • updater direct activ`);
      } catch (err) {
        setUpdateText(`Updater local indisponibil: ${err.message}`);
      }
    };

    window.checkAppUpdate = async function (silent = false) {
      setUpdateText('Verific update-ul…');
      try {
        const d = await getJSONWithReconnect('/api/update/check', 3);
        if (!d.configured) {
          setUpdateText(d.message || 'Updaterul online nu este configurat.');
          return;
        }
        setUpdateText(d.newer
          ? `Update disponibil: ${d.manifest.version} — ${d.manifest.notes || ''}`
          : `Ești la zi • ${d.manifest.version}`);
        if (d.newer && !silent && typeof window.toast === 'function') {
          window.toast(`Versiune nouă: ${d.manifest.version}`);
        }
      } catch (err) {
        const message = String(err?.message || err || 'eroare necunoscută');
        const friendly = /failed to fetch|networkerror|load failed/i.test(message)
          ? 'Conexiunea locală cu DDG s-a întrerupt după 3 încercări. Nu este nevoie să repornești imediat; poți apăsa din nou „Verifică acum”.'
          : message;
        setUpdateText(`Updater indisponibil momentan: ${friendly}`);
        if (!silent && typeof window.toast === 'function') window.toast(friendly);
      }
    };
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', installOverrides, {once: true});
  } else {
    installOverrides();
  }
})();
