/// <reference path="./panel-globals.d.ts" />
'use strict';
// Editor guiado de ACL Headscale. Solo el botón Guardar existente, accionado
// explícitamente, escribe la política remota; el editor no lo hace por sí solo.
(() => {
  const $ = id => /** @type {HTMLInputElement} */ (document.getElementById(id));
  const tr = s => typeof window.tr === 'function' ? window.tr(s) : s;
  let policy = {}, commentsMayBeLost = false, templates = [], selected = null, editingGrant = -1;
  let identityUsers=[], identityNodes=[], undoStack=[];
  const appendValue=(id,value)=>{const field=$(id);field.value=[...new Set([...lines(field.value),value])].join(', ');};
  const userIdentity=u=>{const name=u.email||u.name||'';return name && !name.includes('@')?name+'@':name;};
  function validPort(value){
    const match=/^(?:(tcp|udp|icmp|igmp|sctp|gre|esp|ah|ipv4|ipv6|ip-in-ip|igp|egp|[0-9]{1,3}):)?(\*|[0-9]{1,5}(?:-[0-9]{1,5})?)$/.exec(value);
    if(!match)return false;
    if(match[1] && /^\d+$/.test(match[1]) && (+match[1]<1 || +match[1]>255))return false;
    if(match[2]==='*')return true;
    const [a,b=a]=match[2].split('-').map(Number);return a>=1 && b<=65535 && a<=b;
  }
  const editor = () => $('hs-policy-editor');
  const policyAPI = 'network/admin/policy';
  const libraryAPI = 'network/admin/policy/templates';
  const lines = raw => raw.split(',').map(v => v.trim()).filter(Boolean);
  const tell = (id, text) => { $(id).textContent = tr(text); };
  const download = (filename, data) => {
    const url = URL.createObjectURL(new Blob([data], {type:'application/json;charset=utf-8'}));
    const link = document.createElement('a'); link.href=url;link.download=filename;
    document.body.append(link);link.click();link.remove();setTimeout(()=>URL.revokeObjectURL(url),1500);
  };
  async function parseDraft() {
    const text = editor().value;
    const parsed = await api(`${policyAPI}/parse`, 'POST', {policy:text});
    if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) throw Error(tr('La política no es un objeto huJSON'));
    commentsMayBeLost = /\/\/|\/\*/.test(text);
    policy = parsed;
    editingGrant = -1;
    return parsed;
  }
  function setDraft(value) {
    if (!value || typeof value !== 'object' || Array.isArray(value)) throw Error(tr('Política inválida'));
    if(editor().value.trim()){undoStack.push(editor().value);if(undoStack.length>30)undoStack.shift();}
    editingGrant=-1;
    $('hs-grant-add').textContent=tr('Añadir permiso');
    policy = value;
    editor().value = JSON.stringify(policy, null, 2) + '\n';
    editor().dispatchEvent(new Event('input', {bubbles:true}));
    showVisual();
  }
  function row(container, label, description, onEdit, onDelete) {
    const node=document.createElement('div'); node.className='policy-rule';
    const content=document.createElement('span');
    const strong=document.createElement('strong');strong.textContent=label;
    content.append(strong);
    if (description) {const desc=document.createElement('div');desc.className='field-help';desc.textContent=description;content.append(desc);}
    const actions=document.createElement('div');actions.className='actions';
    const edit=document.createElement('button');edit.type='button';edit.className='secondary';edit.textContent=tr('Editar');edit.addEventListener('click',onEdit);
    const remove=document.createElement('button');remove.type='button';remove.className='secondary';remove.textContent=tr('Quitar');remove.addEventListener('click',onDelete);
    actions.append(edit,remove);node.append(content,actions);container.append(node);
  }
  function showVisual() {
    const groups = $('hs-visual-groups'), tags=$('hs-visual-tags'), grants=$('hs-visual-grants');
    groups.replaceChildren();tags.replaceChildren();grants.replaceChildren();
    const groupMap = policy.groups && typeof policy.groups==='object' && !Array.isArray(policy.groups) ? policy.groups : {};
    const tagMap = policy.tagOwners && typeof policy.tagOwners==='object' && !Array.isArray(policy.tagOwners) ? policy.tagOwners : {};
    for (const [name, members] of Object.entries(groupMap)) {
      row(groups,name,Array.isArray(members)?members.join(', '):tr('Formato no editable'),()=>{
        $('hs-group-name').value=name.replace(/^group:/,'');$('hs-group-members').value=Array.isArray(members)?members.join(', '):'';
      },()=>{delete groupMap[name];policy.groups=groupMap;setDraft(policy)});
    }
    for (const [name, owners] of Object.entries(tagMap)) {
      row(tags,name,Array.isArray(owners)?owners.join(', '):tr('Formato no editable'),()=>{
        $('hs-tag-name').value=name.replace(/^tag:/,'');$('hs-tag-owners').value=Array.isArray(owners)?owners.join(', '):'';
      },()=>{delete tagMap[name];policy.tagOwners=tagMap;setDraft(policy)});
    }
    (Array.isArray(policy.grants)?policy.grants:[]).forEach((grant,index)=>{
      const format = k => Array.isArray(grant[k])?grant[k].join(', '):JSON.stringify(grant[k]||[]);
      row(grants,format('src')+' → '+format('dst'),tr('Puertos: ')+format('ip'),()=>{
        $('hs-grant-src').value=format('src');$('hs-grant-dst').value=format('dst');$('hs-grant-ip').value=format('ip');
        // Conserva la regla anterior hasta que se añada expresamente su sustitución.
        editingGrant = index;
        $('hs-grant-add').textContent=tr('Guardar cambios del permiso');
        tell('hs-policy-status','Editando permiso: al pulsar Añadir permiso se sustituirá esta regla en el borrador.');
      },()=>{policy.grants.splice(index,1);setDraft(policy)});
    });
    const advanced=Object.keys(policy).filter(k=>!['groups','tagOwners','grants'].includes(k));
    const aclCount=Array.isArray(policy.acls)?policy.acls.length:0;
    tell('hs-visual-advanced',advanced.length?tr('Secciones conservadas sin modificar: ')+advanced.join(', ')+(aclCount?`. ${aclCount} ACL(s) heredadas.`:''):tr('No hay secciones avanzadas.'));
    $('hs-visual-warning').hidden=!commentsMayBeLost;
    if (commentsMayBeLost) tell('hs-visual-warning','Atención: al modificar visualmente se reescribe el huJSON como JSON estándar y se pierden los comentarios y el formato original. Las secciones desconocidas se conservan.');
    renderIdentities(groupMap,tagMap);
  }
  function renderIdentities(groupMap=policy.groups||{},tagMap=policy.tagOwners||{}) {
    const data=$('hs-policy-identities'),users=$('hs-policy-users');data.replaceChildren();users.replaceChildren();
    const choices=['*','autogroup:member','autogroup:self','autogroup:tagged','autogroup:internet', ...Object.keys(groupMap),...Object.keys(tagMap),...Object.keys(policy.hosts||{}),...identityUsers.map(userIdentity),...identityNodes.flatMap(n=>[...(n.ip_addresses||[]),...(n.tags||[])])].filter(Boolean);
    for (const value of new Set(choices)){const o=document.createElement('option');o.value=value;data.append(o);}
    for(const u of identityUsers){const value=userIdentity(u);if(!value)continue;const o=document.createElement('option');o.value=value;o.textContent=u.displayName||u.name||value;users.append(o);}
    const pick=$('hs-identity-pick');pick.replaceChildren();
    const filter=$('hs-identity-search').value.trim().toLowerCase();
    const entries=[...identityUsers.map(u=>[userIdentity(u),window.tr("Usuario · {p0}", {p0:((u.name||u.email))})]),...identityNodes.flatMap(n=>(n.ip_addresses||[]).map(ip=>[ip,window.tr("Equipo · {p0} · {p1}", {p0:((n.given_name||n.name)),p1:(ip)})])),...Object.keys(groupMap).map(v=>[v,window.tr("Grupo · {p0}", {p0:(v)})]),...new Set([...Object.keys(tagMap),...identityNodes.flatMap(n=>n.tags||[])])].map(v=>Array.isArray(v)?v:[v,window.tr("Etiqueta · {p0}", {p0:(v)})]);
    for(const [value,label] of entries){if(!value || !(label+' '+value).toLowerCase().includes(filter))continue;const o=document.createElement('option');o.value=value;o.textContent=label;pick.append(o);}
    tell('hs-identity-status',window.tr("{p0} usuarios · {p1} equipos. Selecciona uno o varios y añádelos al campo deseado.", {p0:(identityUsers.length),p1:(identityNodes.length)}));
  }
  function safeName(value,prefix){
    const name=value.trim();if(!/^[a-z0-9][a-z0-9_-]{0,62}$/.test(name)) throw Error(tr('Nombre no válido: solo minúsculas, números, guiones y guion bajo'));
    return prefix+name;
  }
  function addMap(nameField,membersField,section,prefix){
    const key=safeName($(nameField).value,prefix);const members=lines($(membersField).value);
    if (!members.length || members.length>64) throw Error(tr('Indica entre 1 y 64 identidades'));
    policy[section]??={};policy[section][key]=members;setDraft(policy);
  }
  $('hs-group-add').addEventListener('click',()=>run(async()=>addMap('hs-group-name','hs-group-members','groups','group:')));
  $('hs-tag-add').addEventListener('click',()=>run(async()=>addMap('hs-tag-name','hs-tag-owners','tagOwners','tag:')));
  $('hs-grant-add').addEventListener('click',()=>run(async()=>{
    const src=lines($('hs-grant-src').value),dst=lines($('hs-grant-dst').value),ports=lines($('hs-grant-ip').value);
    if (!src.length || !dst.length || !ports.length || ports.some(x=>!validPort(x))) throw Error(tr('Indica origen, destino y puertos válidos (ejemplo: 80,443 o *)'));
    if (Array.isArray(policy.acls) && policy.acls.length && !confirm(tr('Hay reglas ACL heredadas. ¿Añadir también esta regla Grant?')))return;policy.grants??=[];if(editingGrant>=0 && editingGrant<policy.grants.length){policy.grants[editingGrant]={...policy.grants[editingGrant],src,dst,ip:ports};editingGrant=-1;}else{policy.grants.push({src,dst,ip:ports});}setDraft(policy);
  }));
  $('hs-grant-wide').addEventListener('click',()=>run(async()=>{
    if (!confirm(tr('Permitir tráfico sin restricciones es peligroso. ¿Insertar esta regla en el borrador?'))) return;
    policy.grants??=[];policy.grants.push({src:['*'],dst:['*'],ip:['*']});setDraft(policy);
  }));
  $('hs-policy-mode-visual').addEventListener('click',()=>run(async()=>{
    await parseDraft();await refreshIdentities();if (Array.isArray(policy.acls) && policy.acls.length) {if (!confirm(tr('La política tiene ACLs heredadas. El editor visual solo modifica Grants, grupos y etiquetas. ¿Continuar manteniendo las ACL existentes?'))) return;}showVisual();$('hs-policy-visual').hidden=false;$('hs-policy-code-mode').hidden=true;
  }));
  $('hs-policy-mode-code').addEventListener('click',()=>{
    $('hs-policy-visual').hidden=true;$('hs-policy-code-mode').hidden=false;
  });
  $('hs-policy-local-validate').addEventListener('click',()=>run(async()=>{
    await parseDraft();showVisual();tell('hs-policy-status','Sintaxis huJSON válida. La validación definitiva de las reglas se realiza en Headscale al aplicar.');
  }));
  $('hs-policy-compare').addEventListener('click',()=>{
    $('hs-policy-diff').hidden=false;/** @type {HTMLDetailsElement} */ (document.getElementById('hs-policy-diff')).open=true;
    $('hs-policy-before').textContent=window.hgwOriginalACL||tr('Política no cargada');
    $('hs-policy-after').textContent=editor().value;
  });
  document.addEventListener('hgw-policy-loaded', event=>{
    undoStack=[];
    const detail=/** @type {CustomEvent} */ (event).detail||{};if(detail.missing){
      $('hs-policy-initialize').hidden=false;
      $('hs-policy-visual').hidden=true;$('hs-policy-code-mode').hidden=false;
      return;
    }
    $('hs-policy-initialize').hidden=true;
    if (!$('hs-policy-visual').hidden) run(async()=>{await parseDraft();showVisual()});
  });
  async function initPolicy(policyText) {
    if (!confirm(tr('¿Inicializar la política ACL de Headscale? Confirma que tu servidor está en modo database.'))) return;
    if (policyText==='{}' && !confirm(tr('La política {} permite todas las conexiones. ¿Confirmas?'))) return;
    if (policyText!=='{}' && !confirm(tr('ATENCIÓN: bloquear todo puede cortar tus conexiones VPN. ¿Continuar?'))) return;
    const result=await api(policyAPI,'PUT',{policy:policyText,revision:'missing'});
    $('hs-policy-initialize').hidden=true;
    document.dispatchEvent(new CustomEvent('hgw-policy-initialized',{detail:result}));
    notice(tr('Política inicializada. Revisa los cambios antes de aplicar otras reglas.'));
  }
  $('hs-policy-init-open').addEventListener('click',()=>run(async()=>initPolicy('{}')));
  $('hs-policy-init-deny').addEventListener('click',()=>run(async()=>initPolicy('{"grants":[]}')));
  function selectTemplate() {
    selected=templates.find(t=>t.id===$('hs-tpl-select').value)||null;
    tell('hs-tpl-description',tr(selected?.description||''));
    $('hs-tpl-warning').hidden=!selected?.warning;
    tell('hs-tpl-warning',tr(selected?.warning||''));
    $('hs-tpl-remove').disabled=!selected||selected.source==='builtin';
  }
  async function refreshLibrary() {
    const all=await api(libraryAPI);if(!Array.isArray(all)) throw Error(tr('Catálogo no válido'));
    templates=all;const sel=$('hs-tpl-select'),previous=sel.value;sel.replaceChildren();
    for(const item of all){const opt=document.createElement('option');opt.value=item.id;opt.textContent=`${tr(item.category||'General')} · ${tr(item.name)}`;sel.append(opt);}
    if(all.some(t=>t.id===previous))sel.value=previous;
    selectTemplate();
  }
  $('hs-tpl-select').addEventListener('change',selectTemplate);
  $('hs-tpl-refresh').addEventListener('click',()=>run(refreshLibrary));
  $('hs-tpl-use').addEventListener('click',()=>run(async()=>{
    if(!selected)return;
    await window.preparePolicyEditor();
    if(editor().value.trim() && !confirm(tr('¿Sustituir el borrador del editor por esta plantilla? No se aplicará a Headscale.')))return;
    editor().value=selected.policy;
    document.querySelector('[data-tab="policy-dns"]').click();
    $('hs-policy-code-mode').hidden=false;$('hs-policy-visual').hidden=true;
    tell('hs-policy-status','Plantilla cargada solo en el borrador. Revísala antes de guardar.');
    notice(tr('Plantilla cargada; sin cambios en la red.'));
  }));
  $('hs-tpl-save-current').addEventListener('click',()=>run(async()=>{
    const name=await panelDialog({title:tr('Nombre descriptivo de la nueva plantilla:')});if(!name)return;
    const id=await panelDialog({title:tr('Identificador corto (minúsculas y guiones):'),value:name.toLowerCase().normalize('NFD').replace(/[\u0300-\u036f]/g,'').replace(/[^a-z0-9]+/g,'-').replace(/^-+|-+$/g,'').slice(0,50)});if(!id)return;
    await parseDraft();
    const item={id,name,description:tr('Plantilla personal guardada desde el editor'),category:'Personal',warning:tr('Revisa esta política antes de aplicarla'),policy:editor().value};
    const result=await api(`${libraryAPI}/import`,'POST',{format:'headscale-gateway-acl-templates',version:1,templates:[item],overwrite:false});
    if(result.skipped)throw Error(tr('Ya existe una plantilla con ese ID; elige otro nombre'));
    await refreshLibrary();$('hs-tpl-select').value=id;selectTemplate();notice(tr('Plantilla personal guardada en SQLite'));
  }));
  $('hs-tpl-remove').addEventListener('click',()=>run(async()=>{
    if(!selected||selected.source==='builtin')return;
    if(!confirm(tr('¿Eliminar esta plantilla del catálogo local? No afecta a Headscale.')))return;
    await api(`${libraryAPI}/${encodeURIComponent(selected.id)}`,'DELETE');await refreshLibrary();
  }));
  $('hs-tpl-export').addEventListener('click',()=>run(async()=>{
    const items=await api(libraryAPI);download('gateway-acl-templates.json',JSON.stringify({format:'headscale-gateway-acl-templates',version:1,templates:items},null,2)+'\n');
  }));
  $('hs-tpl-import').addEventListener('change',event=>run(async()=>{
    const input=/** @type {HTMLInputElement} */ (event.target),file=input.files?.[0];if(!file)return;
    try {
      if(file.size>64*1024)throw Error(tr('El catálogo supera 64 KiB'));
      const json=JSON.parse(await file.text());if(!Array.isArray(json.templates))throw Error(tr('Catálogo no válido'));
      if(!confirm(tr('¿Importar estas plantillas en SQLite? No se aplicarán a Headscale.')))return;
      await api(`${libraryAPI}/import`,'POST',{...json,overwrite:$('hs-tpl-overwrite').checked});await refreshLibrary();
    }finally{input.value='';}
  }));
  async function refreshRepo() {
    const repo=await api(`${libraryAPI}/repository`);
    $('hs-tpl-repo-url').value=repo.url||'';$('hs-tpl-repo-private').checked=!!repo.allow_private;
  }
  $('hs-tpl-repo-save').addEventListener('click',()=>run(async()=>{
    await api(`${libraryAPI}/repository`,'PUT',{url:$('hs-tpl-repo-url').value.trim(),allow_private:$('hs-tpl-repo-private').checked});
    tell('hs-tpl-repo-status','Configuración del repositorio guardada');
  }));
  $('hs-tpl-repo-preview').addEventListener('click',()=>run(async()=>{
    const bundle=await api(`${libraryAPI}/repository/preview`);
    tell('hs-tpl-repo-status',window.tr("Catálogo disponible: {p0} plantilla(s): {p1}", {p0:(bundle.templates.length),p1:(bundle.templates.map(t=>t.name).join(', '))}));
  }));
  $('hs-tpl-repo-sync').addEventListener('click',()=>run(async()=>{
    if(!confirm(tr('¿Descargar plantillas a la base de datos? No se aplicarán a Headscale.')))return;
    const result=await api(`${libraryAPI}/repository/sync`,'POST',{overwrite:$('hs-tpl-overwrite').checked});
    tell('hs-tpl-repo-status',`Nuevas: ${result.added}. Actualizadas: ${result.updated}. Omitidas: ${result.skipped}.`);
    await refreshLibrary();
  }));
  document.querySelector('[data-tab="policy-templates"]').addEventListener('click',()=>{
    if(currentRole!=='admin')return;
    run(async()=>{await refreshLibrary();await refreshRepo()});
  });
  document.querySelector('[data-tab="policy-dns"]').addEventListener('click',()=>{if(currentRole==='admin')run(refreshIdentities);});
  async function refreshIdentities(){
    const results=await Promise.allSettled([api('network/admin/users'),api('network/nodes')]);
    if(results[0].status==='fulfilled'){const r=results[0].value;identityUsers=Array.isArray(r)?r:(r.users||r.user||[]);}
    if(results[1].status==='fulfilled'){const r=results[1].value;identityNodes=Array.isArray(r)?r:(r.nodes||[]);}
    renderIdentities();renderNodeChoices();
    if(results.some(r=>r.status==='rejected'))tell('hs-identity-status','No se pudo actualizar todo el inventario. Comprueba la API key y usa Actualizar identidades. Se conservan los últimos datos.');
  }
  function renderNodeChoices(){
    const container=$('hs-policy-nodes');container.replaceChildren();
    for(const node of identityNodes){
      const div=document.createElement('div');div.className='policy-rule';
      const label=document.createElement('span');label.textContent=`${node.given_name||node.name} · ${(node.ip_addresses||[]).join(', ')} · ${node.user||'sin usuario'}`;div.append(label);
      for(const ip of node.ip_addresses||[]){for(const [text,id] of [['Origen','hs-grant-src'],['Destino','hs-grant-dst']]){const btn=document.createElement('button');btn.type='button';btn.className='secondary';btn.textContent=text+' · '+ip;btn.addEventListener('click',()=>appendValue(id,ip));div.append(btn);}}
      container.append(div);
    }
    if(!identityNodes.length)tell('hs-policy-nodes','Sin equipos disponibles. Actualiza las identidades o revisa Headscale.');
  }
  $('hs-identity-search').addEventListener('input',()=>renderIdentities());
  $('hs-identity-refresh').addEventListener('click',()=>run(refreshIdentities));
  for(const [button,field] of [['hs-pick-src','hs-grant-src'],['hs-pick-dst','hs-grant-dst'],['hs-pick-group','hs-group-members'],['hs-pick-owner','hs-tag-owners']]){
    $(button).addEventListener('click',()=>{for(const option of $('hs-identity-pick').selectedOptions){if(field==='hs-group-members'&&!identityUsers.some(u=>userIdentity(u)===option.value)){tell('hs-identity-status','Los grupos solo admiten usuarios; selecciona entradas de tipo Usuario.');continue;}appendValue(field,option.value);}});
  }
  $('hs-service-preset').addEventListener('change',()=>{if($('hs-service-preset').value)$('hs-grant-ip').value=$('hs-service-preset').value;});
  $('hs-grant-cancel').addEventListener('click',()=>{editingGrant=-1;$('hs-grant-add').textContent=tr('Añadir permiso');$('hs-grant-src').value='';$('hs-grant-dst').value='';tell('hs-policy-status','Edición de permiso cancelada; la regla original se conserva.');});
  $('hs-policy-undo').addEventListener('click',()=>run(async()=>{const previous=undoStack.pop();if(!previous)return;editor().value=previous;await parseDraft();editor().dispatchEvent(new Event('input',{bubbles:true}));showVisual();}));
})();
