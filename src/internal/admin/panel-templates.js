'use strict';
// Editor de plantillas: la persistencia reside en el servidor y exige control de roles.
async function loadTemplateEditor() {
 templateTransferPermissions();
 await refreshTemplateRepository();
 const templates=await api('templates/details');const rows=el('template-rows');rows.replaceChildren();
 for(const item of templates){
  const tr=document.createElement('tr'),name=document.createElement('td');name.textContent=item.name;
  const category=document.createElement('td');category.textContent=item.category||'General';
  const description=document.createElement('td');description.textContent=item.description||'—';
  const actions=document.createElement('td');
  if(currentRole!=='viewer')actions.append(button('Editar',()=>openTemplateEditor(item),'secondary'));
  if(currentRole!=='viewer')actions.append(button('Eliminar',()=>run(async()=>{
   if(!confirm(window.tr("¿Eliminar la plantilla {p0}?", {p0:(item.name)})))return;
   await api('templates/'+encodeURIComponent(item.name),'DELETE');await loadTemplateEditor();notice('Plantilla eliminada');
  }),'secondary'));
  tr.append(name,category,description,actions);rows.append(tr);
 }
}
function openTemplateEditor(item){
 if(currentRole==='viewer')return;
 el('template-editor').hidden=false;
 el('template-name').value=item?.name||'';
 el('template-name').readOnly=!!item;
 el('template-json').value=JSON.stringify(item?.proxy||{},null,2);
 el('template-category').value=item?.category||'';
 el('template-description').value=item?.description||'';
 el('template-backend-hint').value=item?.backend_hint||'';
}
el('new-template').addEventListener('click',()=>openTemplateEditor(null));
el('template-cancel').addEventListener('click',()=>{el('template-editor').hidden=true;});
el('template-form').addEventListener('submit',ev=>{ev.preventDefault();run(async()=>{
 const name=el('template-name').value.trim();
 const proxy=JSON.parse(el('template-json').value);
 if(!proxy||typeof proxy!=='object'||Array.isArray(proxy))throw Error('Se requiere un objeto JSON');
 await api('templates/'+encodeURIComponent(name),'PUT',{name,proxy,
   category:el('template-category').value.trim(),
   description:el('template-description').value.trim(),
   backend_hint:el('template-backend-hint').value.trim()});
 el('template-editor').hidden=true;await loadTemplateEditor();notice('Plantilla guardada en SQLite');
});});

// Importar y exportar JSON usa la API autenticada; no se evalúa JavaScript.
async function refreshTemplateRepository(){
 if(currentRole!=='admin')return;
 const repo=await api('templates/repository');
 el('template-repository-url').value=repo.url||'';
 el('template-repository-private').checked=!!repo.allow_private;
}
function templateTransferPermissions(){
 const canManage=currentRole==='admin';
 for(const id of ['template-import','template-export','template-repository-save','template-repository-preview','template-repository-sync'])el(id).disabled=!canManage;
 el('template-transfer').hidden=!canManage;
}
function templateTransferResult(result){
 return `Plantillas: ${result.added} nuevas, ${result.updated} actualizadas, ${result.skipped} omitidas.`;
}
async function templatesChanged(){await loadTemplateEditor();await refresh();}
el('template-import').addEventListener('click',()=>run(async()=>{
 const file=el('template-import-file').files[0];
 if(!file)throw Error('Selecciona un archivo JSON');
 if(file.size>8*1024*1024)throw Error('El archivo supera 8 MiB');
 const parsed=JSON.parse(await file.text());
 if(parsed.format!=='headscale-gateway-templates'||parsed.version!==1||!Array.isArray(parsed.templates))throw Error('Formato de plantillas no admitido');
 const overwrite=el('template-overwrite').checked;
 if(!confirm(window.tr("¿Importar {p0} plantillas{p1}?", {p0:(parsed.templates.length),p1:(overwrite?' y sobrescribir las existentes':'')})))return;
 const result=await api('templates/import','POST',{...parsed,overwrite});
 await templatesChanged();notice(templateTransferResult(result));
}));
el('template-export').addEventListener('click',()=>run(async()=>{
 const data=await api('templates/export');
 const link=document.createElement('a');link.href=URL.createObjectURL(new Blob([JSON.stringify(data,null,2)],{type:'application/json'}));
 link.download='headscale-gateway-templates.json';link.click();setTimeout(()=>URL.revokeObjectURL(link.href),1000);
 notice('Plantillas exportadas');
}));
el('template-repository-save').addEventListener('click',()=>run(async()=>{
 await api('templates/repository','PUT',{url:el('template-repository-url').value.trim(),allow_private:el('template-repository-private').checked});
 notice('Repositorio guardado en SQLite');
}));
el('template-repository-preview').addEventListener('click',()=>run(async()=>{
 const data=await api('templates/repository/preview');
 el('template-repository-status').textContent=`Repositorio accesible: ${data.templates.length} plantillas (${data.templates.map(t=>t.name).join(', ')}).`;
}));
el('template-repository-sync').addEventListener('click',()=>run(async()=>{
 const overwrite=el('template-overwrite').checked;
 if(!confirm(window.tr("¿Descargar e importar plantillas del repositorio{p0}?", {p0:(overwrite?' sobrescribiendo las existentes':'')})))return;
 const result=await api('templates/repository/sync','POST',{overwrite});
 await templatesChanged();notice(templateTransferResult(result));
}));
