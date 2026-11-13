async function refreshBackupOverview(data) {
  if(currentRole!=='admin')return;
  const summary=document.getElementById('overview-backups-summary'),schedule=document.getElementById('overview-backups-schedule');
  try{
    data=data||await api('backups');
    const items=[...(data.items||[])].sort((a,b)=>Date.parse(b.created_at)-Date.parse(a.created_at));
    const latest=items[0],settings=data.settings||{};
    summary.textContent=latest?window.tr("Última copia: {p0} · {p1} · {p2} MiB · {p3} copias disponibles", {p0:(new Date(latest.created_at).toLocaleString()),p1:(latest.id),p2:((latest.bytes/1048576||0).toFixed(2)),p3:(items.length)}):'Todavía no hay copias de seguridad.';
    schedule.textContent=(data.encryption_ready?'Cifrado configurado. ':'Cifrado pendiente de configurar. ')+(settings.interval_hours?window.tr("Automáticas cada {p0} h; conservar {p1} copias.", {p0:(settings.interval_hours),p1:(settings.keep)}):'Copias automáticas desactivadas.');
  }catch(e){summary.textContent=window.tr("No se pudo consultar la última copia: {p0}", {p0:(e.message)});schedule.textContent='';}
}
document.getElementById('overview-backups-open').addEventListener('click',()=>document.querySelector('[data-tab="backups"]').click());
'use strict';
// Passwords never go to localStorage/sessionStorage, URL parameters or logs.
(() => {
  const $=id=>document.getElementById(id);
  const input=id=>/** @type {HTMLInputElement} */($(id));
  let loaded=false;
  const tr=s=>typeof window.tr==='function'?window.tr(s):s;
  function parts(kind){
    return {sqlite:input(`backup-${kind}-sqlite`).checked,acl:input(`backup-${kind}-acl`).checked,
      dns:input(`backup-${kind}-dns`).checked,identity:input(`backup-${kind}-identity`).checked,
      full:input(`backup-${kind}-full`).checked};
  }
  function setParts(kind,values){for(const key of ['sqlite','acl','dns','identity','full'])input(`backup-${kind}-${key}`).checked=!!values[key]}
  function describe(p){return [p.full?'Todos los archivos de datos del Gateway':null,p.sqlite||p.full?'Configuración y servicios (SQLite), con clave de cifrado':null,p.identity||p.full?'Identidad VPN del Gateway (tsnet)':null].filter(Boolean)}
  const aclLabel=value=>({included:'Política ACL: copia incluida',missing:'Política ACL: Headscale no tenía una política almacenada',not_requested:'Política ACL: no solicitada'}[value]||window.tr("Política ACL: {p0}", {p0:((value||'estado desconocido'))}));
  const dnsLabel=value=>({generated_export:'DNS: exportación de nombres de servicios (MagicDNS+); no es una copia del servidor DNS',not_requested:'DNS: no solicitado'}[value]||'DNS: '+(value||'estado desconocido'));
  const size=bytes=>bytes<1024?bytes+' B':bytes<1048576?(bytes/1024).toFixed(1)+' KiB':(bytes/1048576).toFixed(2)+' MiB';

  async function readRestoreStatus(){
    try {
      const state=await api('backups/restore/status');
      const view=$('backup-restore-status');
      if(!view)return;
      if(state.phase==='idle') {view.textContent=tr('Sin restauraciones recientes');return}
      const status={verified:'Copia verificada. Preparando reinicio',applying:'Restauración en curso',
        completed:'Restauración completada',failed:'Restauración fallida'}[state.phase]||state.phase;
      view.textContent=`${tr(status)} · ${state.updated_at||''} · ${state.detail||''}`;
    } catch (_) {} // Can be temporarily unavailable during process replacement.
  }
  async function readBackups(){
    if(currentRole!=='admin')return;
    try{
      const data=await api('backups');const opts=data.settings||{};
      await refreshBackupOverview(data);
      await readRestoreStatus();
      $('backup-encryption-status').textContent=data.encryption_ready?tr('Cifrado configurado (contraseña necesaria para restaurar).'):tr('Primero establece una contraseña de recuperación (mínimo 12 caracteres).');
      /** @type {HTMLButtonElement} */($('backup-key-setup')).disabled=!!data.encryption_ready;
      if(!loaded){input('backup-interval').value=String(opts.interval_hours||0);input('backup-keep').value=opts.keep||7;setParts('auto',opts.components||{sqlite:true});loaded=true}
      const rows=$('backup-rows');rows.replaceChildren();const entries=data.items||[];
      if(!entries.length){const row=document.createElement('tr');const cell=document.createElement('td');cell.colSpan=4;cell.textContent=tr('No hay copias cifradas todavía');row.append(cell);rows.append(row)}
      for(const entry of entries){
        const row=document.createElement('tr');const cell=text=>{const td=document.createElement('td');td.textContent=text;row.append(td);return td};
        const date=cell(entry.created_at?new Date(entry.created_at).toLocaleString():'Fecha desconocida');
        const metadata=document.createElement('details');metadata.className='backup-meta';const label=document.createElement('summary');label.textContent='Identificador de la copia';const identifier=document.createElement('span');identifier.textContent=entry.id;metadata.append(label,identifier);date.append(metadata);
        const contents=cell('');const list=document.createElement('ul');list.className='backup-components';
        for(const text of [...describe(entry.components||{}),aclLabel(entry.acl),dnsLabel(entry.magic_dns)]){const li=document.createElement('li');li.textContent=text;list.append(li)}contents.append(list);
        const integrity=cell(size(entry.bytes||0));const checksum=document.createElement('details');checksum.className='backup-meta';const checksumTitle=document.createElement('summary');checksumTitle.textContent='Huella de integridad SHA-256';const hash=document.createElement('span');hash.textContent=entry.sha256||'No disponible';checksum.append(checksumTitle,hash);integrity.append(checksum);
        const actions=cell(''),actionList=document.createElement('div');actionList.className='backup-actions';actions.append(actionList);
        const button=(label,fn)=>{const b=document.createElement('button');b.type='button';b.textContent=tr(label);b.className='secondary';b.addEventListener('click',()=>run(fn));actionList.append(b)};
        button('Descargar cifrada',async()=>{
          const r=await fetch(window.gatewayURL(`/api/v1/backups/${encodeURIComponent(entry.id)}/download`),{credentials:'same-origin',cache:'no-store'});
          if(!r.ok)throw Error(tr('Descarga no autorizada'));
          const blob=await r.blob();const url=URL.createObjectURL(blob);
          try{const a=document.createElement('a');a.href=url;a.download=`${entry.id}.hgw`;document.body.append(a);a.click();a.remove()}finally{setTimeout(()=>URL.revokeObjectURL(url),1000)}
        });
        button('Guardar en Forgejo',async()=>{
          document.dispatchEvent(new CustomEvent('gateway-forgejo-backup',{detail:entry.id}));
        });
        button('Restaurar desde panel',async()=>{
          if(!confirm(tr('Se reiniciará Gateway. La copia será verificada íntegramente y los archivos anteriores se conservarán. ¿Continuar?')))return;
          const password=input('backup-restore-password').value;
          if(!password)throw Error(tr('Introduce la contraseña de descifrado para restaurar.'));
          const result=await api(`backups/${encodeURIComponent(entry.id)}/restore`,'POST',{password,confirm:'RESTAURAR'});
          input('backup-restore-password').value='';
          $('backup-status').textContent=tr('Copia verificada. Gateway reiniciará y aplicará la restauración.')+` (${result.job})`;
        });
        if(entry.components?.acl||entry.components?.full) button('Restaurar ACL',async()=>{
          if(!confirm(tr('ATENCIÓN: reemplazar las ACL de Headscale puede cortar conexiones de toda la red. ¿Restaurar esta política?')))return;
          if(!confirm(tr('Confirma por segunda vez la restauración de ACL remota.')))return;
          const password=input('backup-restore-password').value;if(!password)throw Error(tr('Introduce la contraseña de la copia.'));
          try{await api(`backups/${encodeURIComponent(entry.id)}/restore-acl`,'POST',{password,confirm:'RESTAURAR ACL'});notice(tr('Política ACL restaurada en Headscale.'))}
          finally{input('backup-restore-password').value=''}
        });
        if(entry.components?.dns||entry.components?.full) button('Exportar DNS',async()=>{
          const password=input('backup-restore-password').value;if(!password)throw Error(tr('Introduce la contraseña de la copia.'));
          try{const r=await fetch(window.gatewayURL(`/api/v1/backups/${encodeURIComponent(entry.id)}/dns-export`),{method:'POST',headers:{'Content-Type':'application/json',...(csrf?{'X-CSRF-Token':csrf}:{})},body:JSON.stringify({password}),credentials:'same-origin'});
            if(!r.ok)throw Error(tr('No se pudo exportar la instantánea DNS'));
            const url=URL.createObjectURL(await r.blob());try{const a=document.createElement('a');a.href=url;a.download='magicdns-extra-records.json';document.body.append(a);a.click();a.remove()}finally{URL.revokeObjectURL(url)}
          }finally{input('backup-restore-password').value=''}
        });
        button('Eliminar',async()=>{if(!confirm(tr('¿Eliminar permanentemente la copia cifrada?')))return;await api(`backups/${encodeURIComponent(entry.id)}`,'DELETE',{});await readBackups()});
        rows.append(row);
      }
      $('backup-status').textContent=`${entries.length} ${tr('copias cifradas')} · ${opts.interval_hours?tr('cada')+' '+opts.interval_hours+' h':tr('programación desactivada')}`;
    }catch(e){$('backup-status').textContent=e.message}
  }
  $('backup-key-setup').addEventListener('click',()=>run(async()=>{
    const password=input('backup-key-password').value;
    if(password.length<12)throw Error(tr('La contraseña necesita 12 caracteres como mínimo.'));
    if(password!==input('backup-key-confirm').value)throw Error(tr('Las contraseñas no coinciden'));
    if(!confirm(tr('La contraseña será necesaria para restaurar todas las copias. No puede recuperarse. ¿Confirmar?')))return;
    await api('backups/encryption','POST',{password});input('backup-key-password').value='';input('backup-key-confirm').value='';notice(tr('Cifrado configurado'));await readBackups();
  }));
  $('backup-create').addEventListener('click',()=>run(async()=>{
    const password=input('backup-manual-password').value;const components=parts('manual');
    if(!password)throw Error(tr('Introduce la contraseña de cifrado.'));
    if(!Object.values(components).some(Boolean))throw Error(tr('Selecciona al menos un componente.'));
    if(!confirm(tr('¿Crear una copia cifrada con los componentes seleccionados?')))return;
    $('backup-status').textContent=tr('Creando copia cifrada…');try{await api('backups','POST',{password,components});notice(tr('Copia cifrada creada'));await readBackups()}finally{input('backup-manual-password').value=''}
  }));
  $('backup-save-schedule').addEventListener('click',()=>run(async()=>{
    const data={interval_hours:Number(input('backup-interval').value),keep:Number(input('backup-keep').value),components:parts('auto')};
    if(!confirm(tr('¿Guardar la programación y la selección de componentes?')))return;
    await api('backups/schedule','PUT',data);notice(tr('Programación guardada'));await readBackups();
  }));
  $('backup-import').addEventListener('click',()=>run(async()=>{
    const archive=input('backup-import-file').files?.[0];const password=input('backup-import-password').value;
    if(!archive||!password)throw Error(tr('Selecciona una copia .hgw e introduce su contraseña.'));
    if(!confirm(tr('¿Subir y verificar el archivo cifrado? Esto no modificará todavía tu configuración.')))return;
    const progress=/** @type {HTMLProgressElement} */($('backup-import-progress'));progress.hidden=false;progress.value=0;
    const body=new FormData();body.append('password',password);body.append('archive',archive);
    try{
      const data=await new Promise((resolve,reject)=>{
        const xhr=new XMLHttpRequest();xhr.open('POST',window.gatewayURL('/api/v1/backups/import'),true);xhr.withCredentials=true;
        if(csrf)xhr.setRequestHeader('X-CSRF-Token',csrf);
        xhr.upload.onprogress=e=>{if(e.lengthComputable)progress.value=Math.min(99,Math.round(e.loaded*100/e.total))};
        xhr.onerror=()=>reject(Error(tr('Error de red al subir la copia')));
        xhr.onload=()=>{let value;try{value=JSON.parse(xhr.responseText)}catch{return reject(Error(tr('Respuesta inválida del servidor')))};
          if(xhr.status<200||xhr.status>=300)return reject(Error(value.error||`HTTP ${xhr.status}`));resolve(value)};
        xhr.send(body);
      });
      progress.value=100;notice(tr('Copia cifrada importada y verificada'));await readBackups();
    }finally{input('backup-import-password').value='';progress.hidden=true}
  }));
  document.addEventListener('gateway-forgejo-imported',()=>run(readBackups));
  $('backup-refresh').addEventListener('click',()=>run(readBackups));
  document.querySelector('[data-tab="backups"]').addEventListener('click',()=>run(readBackups));
  document.addEventListener('gateway-admin-ready',()=>run(readBackups));
  setInterval(()=>{if(currentRole==='admin'&&!document.hidden&&!document.getElementById('app').hidden)readRestoreStatus()},5000);
})();
