/// <reference path="./panel-globals.d.ts" />
'use strict';
// Solo material público de CA: nunca llegan al navegador claves privadas, tokens
// ni secretos de emparejamiento. Cambiar la confianza del SO exige confirmación interactiva.
(() => {
  const byId = id => document.getElementById(id);
  const message = (text,bad=false) => { const node=byId('pki-message');node.textContent=text;node.style.color=bad?'#fda4af':''; };
  const showError = error => message(error.message || String(error),true);
  const makeButton=(label,fn)=>{const btn=document.createElement('button');btn.type='button';btn.className='secondary';btn.textContent=label;btn.addEventListener('click',()=>{Promise.resolve(fn()).catch(showError);});return btn;};
  const cell=(value)=>{const el=document.createElement('td');el.textContent=value;return el;};
  const date=value=>value?new Date(value).toLocaleString():'—';
  let loaded=false;
  function androidTrustInstructions(){
    message('Android: descarga la CA pública, compara su huella SHA-256 y usa Ajustes > Seguridad > Instalar certificado > CA. La instalación requiere tu consentimiento en Android.');
    window.dispatchEvent(new Event('hgw-android-certificate-settings'));
  }
  async function downloadCA(path,name){
    const response=await fetch(window.gatewayURL('/api/v1/'+path),{headers:{...(token?{Authorization:'Bearer '+token}:{}),'Accept-Language':window.gatewayLanguage?.()||'es'}});
    if(!response.ok)throw Error('No se pudo descargar el certificado');
    const blob=await response.blob();const url=URL.createObjectURL(blob);
    try{const a=document.createElement('a');a.href=url;a.download=name;a.click();}finally{setTimeout(()=>URL.revokeObjectURL(url),2000);}
  }
  const confirmed=async(fp,action)=>(await panelDialog({title:action,message:window.tr("CA SHA-256: {p0}\n\nInstalar una CA raíz concede confianza para cualquier dominio. Al eliminar una CA local en uso, tras reiniciar puede generarse otra y habrá que distribuirla a los equipos.", {p0:(fp)}),expected:fp,accept:'Confirmar operación'}))!==null;
  async function operation(path,fp,description){
    if(!await confirmed(fp,description)){message('Operación cancelada o huella no coincidente.',true);return;}
    try{const result=await api(path,'POST',{fingerprint:fp});await refreshPKI();message(result.local_removed ? window.tr("CA eliminada del almacén del Gateway. {p0} La CA en memoria se mantiene hasta reiniciar; se generará una nueva si el Gateway la necesita.", {p0:((result.uninstalled?'Retirada del SO completada.':window.tr("Retirada del SO pendiente: {p0} {p1}", {p0:(result.warning),p1:(result.instructions)})))}) : 'Operación realizada. Reinicia las aplicaciones que mantengan certificados en memoria.');}
    catch(error){message(error.message,true);return;}
  }
  async function refreshPKI(){
    const data=await api('pki');loaded=true;
    const android=data.platform==='android';
    byId('pki-helper-status').closest('.card').hidden=android;
 byId('pki-helper-enabled').checked=!!data.helper_enabled;
 byId('pki-helper-status').textContent=(data.helper?.available?'Helper disponible':'Helper no instalado')+' · '+(data.helper?.platform||data.platform||'')+' · '+(data.helper?.path||'')+' · '+(data.helper_enabled?'activado':'desactivado');
    byId('pki-public-enabled').checked=!!data.publish_public_ca;
    byId('pki-local-rows').replaceChildren();byId('pki-leaf-rows').replaceChildren();byId('pki-peer-rows').replaceChildren();
    for(const cert of(data.issued_certificates||[])){const row=document.createElement('tr');row.append(cell(cert.domain),cell([...(cert.dns_names||[]),...(cert.ip_addresses||[])].join(', ')),cell(date(cert.expires)+(cert.expired?' · CADUCADO':'')));byId('pki-leaf-rows').append(row);}
    for(const ca of (data.local_ca||[])){
      const tr=document.createElement('tr');tr.append(cell(ca.name),cell(date(ca.expires)+' · '+ca.fingerprint));
      const td=document.createElement('td');td.className='actions';
      td.append(makeButton('Descargar CA pública',()=>downloadCA('pki/ca/'+ca.fingerprint+'/certificate','hgw-local-'+ca.fingerprint+'.crt')),
        makeButton(android?'Instalar en Android':'Instalar en SO',android?androidTrustInstructions:()=>operation('pki/ca/'+ca.fingerprint+'/install',ca.fingerprint,'Instalar esta CA local en el sistema operativo')),
        makeButton('Eliminar del Gateway y SO',()=>operation('pki/ca/'+ca.fingerprint+'/uninstall',ca.fingerprint,'Eliminar esta CA y su clave del Gateway e intentar retirarla del SO. Si está en uso, tras reiniciar se generará otra CA y tendrás que instalarla en tus equipos')));
      tr.append(td);byId('pki-local-rows').append(tr);
    }
    for(const peer of (data.peers||[])){
      const tr=document.createElement('tr');tr.append(cell(peer.name+' · '+peer.fingerprint+'\n'+(peer.endpoint||'')),cell(peer.approved?'Autorizado':'Pendiente de verificar'),cell(date(peer.last_sync)));
      const td=document.createElement('td');td.className='actions';
      if(!peer.approved)td.append(makeButton('Verificar y aprobar',()=>operation('pki/peers/'+peer.fingerprint+'/approve',peer.fingerprint,window.tr("Aprobar la CA pública de {p0}", {p0:(peer.name)}))));
      // Revoke does not uninstall a previously trusted system CA. Always offer explicit removal.
      td.append(makeButton('Retirar del SO',()=>operation('pki/peers/'+peer.fingerprint+'/uninstall',peer.fingerprint,'Retirar esta CA del almacén del sistema')));
      if(peer.approved){
        td.append(makeButton('Sincronizar ahora',async()=>{await api('pki/peers/'+peer.fingerprint+'/sync','POST',{});message('CA remota verificada por HTTPS con huella fijada.');await refreshPKI();}),
          makeButton(android?'Instalar en Android':'Instalar en SO',android?androidTrustInstructions:()=>operation('pki/peers/'+peer.fingerprint+'/install',peer.fingerprint,window.tr("Confiar globalmente en la CA remota de {p0}", {p0:(peer.name)}))),
          makeButton('Revocar autorización',async()=>{if(!await confirmed(peer.fingerprint,window.tr("Revocar confianza en {p0}", {p0:(peer.name)})))return;await api('pki/peers/'+peer.fingerprint,'DELETE');message('Autorización revocada. Si instalaste esta CA en el sistema, debes retirarla aparte.');await refreshPKI();}));
      }else td.append(makeButton('Eliminar',async()=>{if(!confirm('¿Eliminar CA pendiente?'))return;await api('pki/peers/'+peer.fingerprint,'DELETE');await refreshPKI();}));
      td.append(makeButton('Descargar CA',()=>downloadCA('pki/peers/'+peer.fingerprint+'/certificate','hgw-peer-'+peer.fingerprint+'.crt')));
      tr.append(td);byId('pki-peer-rows').append(tr);
    }
    if((data.local_ca||[]).length===0)message('No se ha generado todavía una CA local. No se modificará el primer inicio ni se creará una CA desde este panel.');
  }
  byId('pki-ports-settings').addEventListener('click',()=>document.querySelector('[data-tab="settings"]').click());
  byId('pki-helper-save').addEventListener('click',()=>api('pki/helper','PUT',{enabled:byId('pki-helper-enabled').checked}).then(()=>refreshPKI()).then(()=>message('Preferencia guardada. Cada operación requiere autorización del SO en el equipo del Gateway.')).catch(showError));
  byId('pki-refresh').addEventListener('click',()=>refreshPKI().catch(showError));
  byId('pki-public-save').addEventListener('click',()=>{if(!confirm('¿Cambiar la exposición del certificado público de CA?'))return;api('pki/publication','PUT',{enabled:byId('pki-public-enabled').checked}).then(()=>message('Publicación actualizada. Reinicia Gateway para activar o desactivar sus anuncios LAN.')).catch(showError);});
  byId('pki-peer-add').addEventListener('click',async()=>{
    try{
      const file=byId('pki-peer-file').files?.[0];if(!file||file.size>65536)throw Error('Selecciona un archivo CA público PEM de hasta 64 KiB');
      const name=byId('pki-peer-name').value.trim(),endpoint=byId('pki-peer-url').value.trim();if(!name)throw Error('Introduce un nombre');
      const peer=await api('pki/peers','POST',{name,endpoint,pem:await file.text()});
      message(window.tr("CA registrada como PENDIENTE. Comprueba con el administrador remoto la huella SHA-256 {p0} antes de aprobarla.", {p0:(peer.fingerprint)}));
      byId('pki-peer-file').value='';await refreshPKI();
    }catch(e){showError(e)}
  });
  byId('pki-lan-discover').addEventListener('click',async()=>{
    try{
      const result=await api('pki/discover');const target=byId('pki-discovery');target.replaceChildren();
      const info=document.createElement('p');info.textContent=result.note||'';target.append(info);
      if(result.candidates){
        for(const c of result.candidates){const row=document.createElement('p');row.textContent=c.name+' · '+c.endpoint+' · '+[...(c.targets||[]),...(c.ca_http_targets||[])].map(t=>t.network+' '+t.ip).join(' → ');if(c.error)row.append(document.createTextNode(' · '+c.error));row.append(makeButton('Descargar CA pendiente',()=>downloadPeerCA(c.endpoint)));target.append(row);}
        return;
      }
      const title=document.createElement('strong');title.textContent='Encontrados en LAN (sin verificar):';target.append(title);
      const ul=document.createElement('ul');
      for(const host of result.lan||[]){
        const li=document.createElement('li');li.textContent=host.name+' · '+host.address+':'+host.port+' · '+host.host;
        li.append(makeButton('Utilizar candidato',()=>{byId('pki-peer-name').value=host.name;byId('pki-peer-url').value=host.endpoint||('https://'+host.host+(host.port===443?'':':'+host.port));message('Selecciona el certificado público obtenido por un canal verificado. Descubrir no equivale a confiar.');}));ul.append(li);
      }target.append(ul);
      const candidates=document.createElement('p');candidates.textContent=window.tr("Nodos Headscale detectados: {p0} (no necesariamente ejecutan Gateway).", {p0:((result.tailnet_candidates||[]).map(n=>n.name).join(', '))});target.append(candidates);
    }catch(e){showError(e)}
  });
  async function downloadPeerCA(endpoint){
    const bulk=byId('pki-download-discovered');bulk.disabled=true;
    try{message('Descargando CA públicas: Tailscale primero y LAN después. No se aprueban automáticamente.');
      const results=await api('pki/discover/download','POST',{endpoint});
      const target=byId('pki-discovery');target.replaceChildren();
      for(const item of results){const p=document.createElement('p');p.textContent=item.endpoint+' · '+(item.error||(item.anchor?.approved?window.tr('CA ya aprobada; se conserva su autorización.'):window.tr('CA pendiente: compara su huella antes de aprobar.')))+' '+(item.anchor?.fingerprint||'');target.append(p)}
      await refreshPKI();message('Descarga finalizada. Revisa las huellas y aprueba únicamente las CA verificadas.');
    }catch(e){showError(e)}finally{bulk.disabled=false}
  }
  byId('pki-download-discovered').addEventListener('click',()=>downloadPeerCA(''));
  document.querySelector('[data-tab="pki"]').addEventListener('click',()=>{if(!loaded)refreshPKI().catch(showError);});
})();
