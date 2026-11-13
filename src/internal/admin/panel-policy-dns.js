'use strict';
// Herramientas administrativas para políticas Headscale y exportación MagicDNS.
// Headscale valida la semántica de las políticas; el navegador no la interpreta.
(() => {
  const $ = id => /** @type {HTMLInputElement} */ (document.getElementById(id));
  const translate = s => typeof window.tr === 'function' ? window.tr(s) : s;
  let revision = '', originalPolicy = '', dnsRecords = null, policyLoaded=false;
  window.preparePolicyEditor=async()=>{if(!policyLoaded)await readPolicy();};
  const status = (id, text) => { $(id).textContent = translate(text); };
  const download = (filename, data, type) => {
    const url = URL.createObjectURL(new Blob([data], {type}));
    const anchor = document.createElement('a'); anchor.href = url; anchor.download = filename;
    document.body.append(anchor); anchor.click(); anchor.remove();
    setTimeout(() => URL.revokeObjectURL(url), 1500);
  };
  async function readPolicy() {
    const response = await api('network/admin/policy');
    policyLoaded=true;
    originalPolicy = response.policy || '';
    revision = response.revision || '';
    $('hs-policy-editor').value = response.missing ? '{}' : originalPolicy;
    window.hgwOriginalACL = response.missing ? '(sin política almacenada)' : originalPolicy;
    document.dispatchEvent(new CustomEvent('hgw-policy-loaded',{detail:response}));
    $('hs-policy-editor').disabled = false;
    $('hs-policy-save').disabled = !revision || response.missing;
    $('hs-policy-export').disabled = !revision || response.missing;
    status('hs-policy-status', response.missing ? 'No hay política ACL en la base de datos. Utiliza Inicializar política antes de editar.' : 'Política leída desde Headscale. Guarda una copia de seguridad antes de editar.');
  }
  document.addEventListener('hgw-policy-initialized',()=>run(readPolicy));
  $('hs-policy-refresh').addEventListener('click', () => run(readPolicy));
  $('hs-policy-export').addEventListener('click', () => {
    if (!revision) return;
    download('headscale-policy-backup.hujson', originalPolicy, 'application/json;charset=utf-8');
  });
  $('hs-policy-import').addEventListener('change', event => run(async () => {
    const input = /** @type {HTMLInputElement} */ (event.target);
    const file = input.files?.[0];
    if (!file) return;
    if (file.size > 100 * 1024) throw Error(translate('El archivo de política supera 100 KiB'));
    $('hs-policy-editor').value = await file.text();
    status('hs-policy-status', 'Política importada en el editor. Todavía no se ha aplicado.');
    input.value = '';
  }));
  $('hs-policy-save').addEventListener('click', () => run(async () => {
    if (!revision || revision === 'missing' || currentRole !== 'admin') return;
    const policy = $('hs-policy-editor').value;
    if (!policy.trim()) throw Error(translate('No se permite guardar una política vacía'));
    if (new TextEncoder().encode(policy).length > 100 * 1024) throw Error(translate('La política supera 100 KiB'));
    if (!confirm(translate('¿Reemplazar TODA la política ACL de Headscale? Puedes perder acceso entre dispositivos.'))) return;
    // La confirmación es explícita porque una política vacía permite todo el tráfico.
    if (/^\s*\{\s*\}\s*$/.test(policy) && !confirm(translate('Una política {} permite todo el tráfico. ¿Confirmas el cambio?'))) return;
    const result = await api('network/admin/policy', 'PUT', {policy, revision});
    revision = result.revision;
    originalPolicy = result.policy;
    $('hs-policy-editor').value = originalPolicy;
    document.dispatchEvent(new CustomEvent('hgw-policy-loaded',{detail:result}));
    status('hs-policy-status', 'Política guardada y recuperada de Headscale.');
    notice(translate('Política ACL actualizada'));
  }));
  async function readRecords() {
    const records = await api('network/admin/dns/records');
    if (!Array.isArray(records)) throw Error(translate('Formato DNS no reconocido'));
    dnsRecords = records;
    $('hs-dns-export').disabled = false;
    $('hs-dns-copy').disabled = false;
    const rows = $('hs-dns-rows'); rows.replaceChildren();
    for (const record of records) {
      const tr = document.createElement('tr');
      for (const field of ['name','type','value']) {
        const td = document.createElement('td'); td.textContent = record[field] || ''; tr.append(td);
      }
      rows.append(tr);
    }
    status('hs-dns-status', 'Registros generados. Descárgalos y sincronízalos con el archivo extra_records_path de Headscale.');
  }
  $('hs-dns-refresh').addEventListener('click', () => run(readRecords));
  $('hs-dns-export').addEventListener('click', () => {
    if (!dnsRecords) return;
    download('gateway-extra-records.json', JSON.stringify(dnsRecords, null, 2) + '\n', 'application/json;charset=utf-8');
  });
  $('hs-dns-copy').addEventListener('click', () => {
    if (!dnsRecords) return;
    GenUICopyFallback(JSON.stringify(dnsRecords, null, 2) + '\n');
  });
  async function GenUICopyFallback(content) {
    try { await navigator.clipboard.writeText(content); notice(translate('Registros copiados al portapapeles')); }
    catch { notice(translate('No se pudo copiar. Utiliza Descargar JSON.'), true); }
  }
  document.querySelector('[data-tab="policy-dns"]').addEventListener('click', () => {
    if (currentRole === 'admin' && !policyLoaded) run(readPolicy);
  });
  document.querySelector('[data-tab="magicdns"]').addEventListener('click', () => {
    if (currentRole === 'admin') run(readRecords);
  });
})();
