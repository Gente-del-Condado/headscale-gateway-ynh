'use strict';
// La exposición de API y el control de roles proceden del esquema y se guardan en SQLite.
const exposureFields={
  token_read:'Estado y consultas',token_services:'Servicios y plantillas',
  token_settings:'Configuración general',token_network:'Inventario y diagnóstico de red',
  token_operations:'Recarga y reinicio',token_identity:'Identidad, MFA y políticas'
};
function parseRoleGroups(text){
  const result={};for(const line of text.split(/\r?\n/)){
    const v=line.trim();if(!v)continue;
    const idx=v.indexOf('=');if(idx<1)throw Error(window.tr("Asignación incorrecta: {p0}", {p0:(v)}));
    const group=v.slice(0,idx).trim(),role=v.slice(idx+1).trim();
    if(!group||!['admin','operator','viewer'].includes(role))throw Error(window.tr("Rol incorrecto: {p0}", {p0:(v)}));
    result[group]=role;
  }return result;
}
async function loadExposure(){
  const data=await api('api/exposure');const box=el('exposure-options');box.replaceChildren();
  for(const [key,label] of Object.entries(exposureFields)){
    const line=document.createElement('label');line.className='check';
    const input=document.createElement('input');input.type='checkbox';input.dataset.exposure=key;input.checked=!!data[key];
    line.append(input,document.createTextNode(' '+label));box.append(line);
  }
}
el('exposure-form').addEventListener('submit',ev=>{ev.preventDefault();run(async()=>{
  const next={};el('exposure-options').querySelectorAll('[data-exposure]').forEach(i=>{next[i.dataset.exposure]=i.checked;});
  if(!confirm('Las restricciones a tokens Bearer pueden afectar a clientes externos. ¿Guardar?'))return;
  await api('api/exposure','PUT',next);notice('Política de API guardada en SQLite');
});});
el('exposure-reload').addEventListener('click',()=>run(loadExposure));
let auditPage=1, auditPages=1;
/** @param {string} id */
const auditInput = id => /** @type {HTMLInputElement} */ (el(id));
async function loadAudit(){
  const params=new URLSearchParams({page:String(auditPage),size:auditInput('audit-size').value,sort:auditInput('audit-sort').value,order:auditInput('audit-order').value});
  const result=await api('audit?'+params.toString());const rows=el('audit-rows');rows.replaceChildren();
  for(const evt of result.events||[]){const row=document.createElement('tr');for(const name of ['time','actor','action','outcome']){
    const cell=document.createElement('td');cell.textContent=evt[name]||'';row.append(cell);
  }rows.append(row);}
  auditPages=Math.max(1,Math.ceil((result.total||0)/Number(result.size||10)));
  el('audit-page-info').textContent=window.tr("Página {p0} de {p1} · {p2} eventos", {p0:(auditPage),p1:(auditPages),p2:(result.total||0)});
  auditInput('audit-prev').disabled=auditPage<=1;auditInput('audit-next').disabled=auditPage>=auditPages;
}
el('audit-refresh').addEventListener('click',()=>run(loadAudit));
for(const id of ['audit-size','audit-sort','audit-order'])el(id).addEventListener('change',()=>{auditPage=1;run(loadAudit);});
el('audit-prev').addEventListener('click',()=>{if(auditPage>1){auditPage--;run(loadAudit);}});
el('audit-next').addEventListener('click',()=>{if(auditPage<auditPages){auditPage++;run(loadAudit);}});
el('mfa-setup').addEventListener('click',()=>run(async()=>{
  const data=await api('auth/mfa/setup','POST',{});
  el('mfa-secret').textContent=data.secret+'\n'+data.otpauth_url;
  el('mfa-enrollment').hidden=false;
  notice('Configura el autenticador y confirma un código válido');
}));
el('mfa-confirm').addEventListener('click',()=>run(async()=>{
  await api('auth/mfa/confirm','POST',{code:el('mfa-code').value.trim()});
  el('mfa-secret').textContent='';el('mfa-code').value='';el('mfa-enrollment').hidden=true;
  await refreshAuthSettings();notice('Doble factor activado');
}));
el('mfa-disable').addEventListener('click',()=>run(async()=>{
  const code=await panelDialog({title:'Introduce el código de tu autenticador para desactivar TOTP:'});if(!code)return;
  await api('auth/mfa/disable','POST',{code:code.trim()});await refreshAuthSettings();notice('Doble factor desactivado');
}));

// Tokens con alcances: el secreto se muestra una sola vez y nunca vuelve a listarse.
async function loadAPITokens(){
  const list=await api('api/tokens'),rows=el('api-token-rows');rows.replaceChildren();
  for(const item of list){
    const tr=document.createElement('tr');
    for(const val of [item.id,item.label,item.role,(item.scopes||[]).join(', '),item.expires]){
      const td=document.createElement('td');td.textContent=val;tr.append(td);
    }
    const td=document.createElement('td');td.append(button('Revocar',()=>run(async()=>{
      if(!confirm(window.tr("¿Revocar permanentemente el token {p0}?", {p0:(item.label)})))return;
      await api('api/tokens/'+item.id,'DELETE');await loadAPITokens();notice('Token revocado');
    }),'secondary'));tr.append(td);rows.append(tr);
  }
}
const updateDelegateScopeControls=()=>{
  const canReload=el('api-token-role').value==='operator';
  const reload=el('api-token-scopes').querySelector('input[value="operations"]');
  reload.disabled=!canReload;
  if(!canReload)reload.checked=false;
};
el('api-token-role').addEventListener('change',updateDelegateScopeControls);
updateDelegateScopeControls();
el('new-api-token-form').addEventListener('submit',ev=>{
  ev.preventDefault();run(async()=>{
    const body={label:el('api-token-label').value.trim(),role:el('api-token-role').value,
      days:Number(el('api-token-days').value),scopes:[...el('api-token-scopes').querySelectorAll('input:checked')].map(e=>e.value)};
    const data=await api('api/tokens','POST',body);
    el('new-api-token-value').textContent=data.token;
    el('new-api-token-result').hidden=false;
    await loadAPITokens();notice('Token delegado creado. Guarda el secreto ahora.');
  });
});
el('new-api-token-hide').addEventListener('click',()=>{
  el('new-api-token-value').textContent='';el('new-api-token-result').hidden=true;
});

// Muestra el secreto rotado una sola vez; nunca lo almacena en el navegador.
el('recovery-rotate').addEventListener('click',()=>run(async()=>{
  if(!confirm('Esto revocará de inmediato el token de recuperación anterior. ¿Continuar?'))return;
  const typed=await panelDialog({title:'Para confirmar escribe ROTAR (mayúsculas):'});
  if(typed!=='ROTAR')return;
  const data=await api('auth/recovery/rotate','POST',{confirm:'ROTAR'});
  el('recovery-token-value').textContent=data.token;
  el('recovery-result').hidden=false;
  notice('Token de recuperación rotado. Guarda el nuevo en un lugar seguro.');
}));
el('recovery-hide').addEventListener('click',()=>{
  el('recovery-token-value').textContent='';
  el('recovery-result').hidden=true;
});
