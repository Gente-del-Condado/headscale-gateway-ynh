'use strict';
// Acciones independientes de importar/exportar configuración y diagnóstico LAN.
// Ninguna de estas herramientas toca el directorio de identidad Tailscale.
(() => {
  const byId = id => document.getElementById(id);
  const result = (text, bad=false) => {
    const node=byId('portable-message');
    node.textContent=text;
    node.style.color=bad?'#fda4af':'';
  };
  const password = id => /** @type {HTMLInputElement} */ (byId(id)).value;
  const bytesToBase64 = bytes => {
    let value='';
    for(let i=0;i<bytes.length;i+=8192) value+=String.fromCharCode(...bytes.subarray(i,i+8192));
    return btoa(value);
  };
  byId('config-portable-export').addEventListener('click', async () => {
    try {
      const pass=password('config-portable-password');
      if(pass.length<12) throw Error('La contraseña debe tener al menos 12 caracteres');
      const secrets=(/** @type {HTMLInputElement} */ (byId('config-portable-secrets'))).checked;
      if(secrets && !confirm('El archivo cifrado incluirá claves de API de Headscale y DNS. ¿Continuar?')) return;
      const value=await api('config/transfer/export','POST',{password:pass,secrets});
      const raw=atob(value.data);
      const bytes=Uint8Array.from(raw,c=>c.charCodeAt(0));
      const url=URL.createObjectURL(new Blob([bytes],{type:'application/octet-stream'}));
      const link=document.createElement('a');link.href=url;link.download=value.filename||'gateway.hgwcfg';link.click();
      setTimeout(()=>URL.revokeObjectURL(url),1200);
      result('Perfil cifrado descargado. Guarda su contraseña por separado.');
    }catch(e){result(e.message,true);}
  });
  byId('config-portable-import').addEventListener('click',async()=>{
    try {
      const file=(/** @type {HTMLInputElement} */ (byId('config-portable-file'))).files?.[0];
      if(!file || file.size>320*1024) throw Error('Selecciona un archivo .hgwcfg cifrado válido (máximo 320 KiB)');
      const pass=password('config-portable-password');
      if(!pass) throw Error('Introduce la contraseña de la copia');
      if(!confirm('Importar los ajustes y claves del perfil. Se conservarán el acceso local y los puertos de este equipo. ¿Continuar?'))return;
      const data=bytesToBase64(new Uint8Array(await file.arrayBuffer()));
      const reply=await api('config/transfer/import','POST',{password:pass,data});
      result(window.tr("Importadas {p0} opciones. Reinicia el Gateway para aplicarlas.", {p0:(reply.imported)}));
      window.markRestartRequired?.();
    }catch(e){result(e.message,true);}
  });
  // Un aviso no bloqueante en Resumen sirve también durante el primer acceso.
  const summary=byId('firewall-overview-warning');
  async function reviewOnOverview() {
    try {
      const data=await api('system/firewall');
      summary.hidden=!data.remote || data.status==='permitido';
      if(!summary.hidden)byId('firewall-overview-summary').textContent=window.tr("HTTPS LAN {p0}: {p1}. No se cambiará ninguna regla automáticamente.", {p0:(data.listener),p1:(data.note||data.status)});
    }catch {summary.hidden=true;}
  }
  document.querySelector('[data-tab="overview"]').addEventListener('click',()=>void reviewOnOverview());
  byId('firewall-overview-settings').addEventListener('click',()=>{
    document.querySelector('[data-tab="settings"]').click();
    byId('firewall-refresh').click();
  });
  // La respuesta inicial del panel puede tardar en autenticar Dex; volverá
  // a consultarse al entrar en Resumen, sin mostrar errores de sesión.
  setTimeout(()=>void reviewOnOverview(), 1500);
  byId('firewall-refresh').addEventListener('click',async()=>{
    const report=byId('firewall-report');
    try {
      const data=await api('system/firewall');
      report.replaceChildren();
      const text=`Sistema: ${data.platform||'local'} · Listener LAN: ${data.listener} · Estado: ${data.status}\n${data.note||''}\n${(data.instructions||[]).join('\n')}`;
      report.textContent=text;
    }catch(e){report.textContent=e.message;}
  });
})();
