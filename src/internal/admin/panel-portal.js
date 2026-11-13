'use strict';
(()=>{
 const $=id=>document.getElementById('portal-'+id);let config=null;
 const tr=(s,p)=>window.tr(s,p);
 const message=s=>$('message').textContent=s;
 const safe=fn=>async()=>{try{await fn()}catch(e){message(e.message)}};
 const fields=['title','description','accent','theme'],flags=['enabled','search','show-admin','publish','federation','discovery'];
 function text(tag,value){const e=document.createElement(tag);e.textContent=value;return e;}
 function fill(data,keepDraft=false){
 if(!keepDraft){
  config=structuredClone(data.config);for(const id of fields)$(id).value=config[id]||'';
  for(const id of flags)$(id).checked=!!config[id.replaceAll('-','_')];
  $('open').href=data.url;$('local').replaceChildren();
  for(const name of data.local_names||[]){
   const value=config.local?.[name]||{},row=document.createElement('tr');row.dataset.name=name;row.append(text('td',name));
   for(const key of ['label','description','group']){const cell=document.createElement('td'),input=document.createElement('input');input.dataset.key=key;input.value=value[key]||'';input.maxLength=key==='description'?400:key==='group'?60:100;input.setAttribute('aria-label',name+' · '+key);cell.append(input);row.append(cell)}
   for(const key of ['visible','share']){const cell=document.createElement('td'),input=document.createElement('input');input.type='checkbox';input.dataset.key=key;input.checked=key==='visible'?!value.hidden:!!value.share;input.setAttribute('aria-label',name+' · '+key);cell.append(input);row.append(cell)}
   $('local').append(row);
  }
  }
  $('peers').replaceChildren();
  if(!(data.peers||[]).length)$('peers').append(text('p',tr('No hay gateways aprobados con un portal configurado.')));
  for(const peer of data.peers||[]){
   const box=document.createElement('fieldset');box.append(text('legend',peer.name||peer.endpoint),text('p',peer.catalog?.origin||peer.endpoint));
   box.append(text('p',peer.error|| (peer.checked&&!peer.checked.startsWith('0001-')?tr('Última comprobación: {time}',{time:new Date(peer.checked).toLocaleString()}):tr('Catálogo pendiente de consultar'))));
   const entries=[...(peer.catalog?.services||[])];
   for(const selected of config.selected||[])if(selected.peer===peer.peer&&!entries.some(e=>e.id===selected.id))entries.push({id:selected.id,name:selected.id+' · '+tr('No disponible')});
   for(const entry of entries){const label=document.createElement('label'),input=document.createElement('input');input.type='checkbox';input.dataset.peer=peer.peer;input.dataset.service=entry.id;input.checked=(config.selected||[]).some(s=>s.peer===peer.peer&&s.id===entry.id);label.append(input,document.createTextNode(entry.name+' · '+(entry.url||'')));box.append(label)}
   $('peers').append(box);
  }
 }
 async function refresh(){fill(await api('portal'))}
 $('form').onsubmit=event=>{event.preventDefault();void safe(async()=>{
  if(!config)return;
  const next=structuredClone(config);for(const id of fields)next[id]=$(id).value;for(const id of flags)next[id.replaceAll('-','_')]=$(id).checked;
  for(const kind of ['logo','favicon']){const file=$(kind).files[0];if(file){if(file.type!=='image/png'||file.size>128*1024)throw Error(tr('Selecciona un PNG de hasta 128 KiB'));const bytes=new Uint8Array(await file.arrayBuffer());let value='';for(const b of bytes)value+=String.fromCharCode(b);next[kind]=btoa(value)}}
  next.local={};for(const row of $('local').children){const v={};for(const input of row.querySelectorAll('input'))v[input.dataset.key]=input.type==='checkbox'?input.checked:input.value;v.hidden=!v.visible;delete v.visible;next.local[row.dataset.name]=v;}
  next.selected=[...$('peers').querySelectorAll('input:checked')].map(i=>({peer:i.dataset.peer,id:i.dataset.service}));
  await api('portal','PUT',next);await refresh();$('logo').value='';$('favicon').value='';message(tr('Portal guardado. Los cambios ya están aplicados.'));
 })()};
 $('refresh').onclick=safe(refresh);
 $('sync').onclick=safe(async()=>{message(tr('Consultando catálogos…'));const data=await api('portal/sync','POST',{});config.selected=[...$('peers').querySelectorAll('input:checked')].map(i=>({peer:i.dataset.peer,id:i.dataset.service}));fill(data,true);message(tr('Catálogos actualizados. Marca los servicios deseados y guarda el portal.'))});
 $('discover').onclick=safe(async()=>{
  message(tr('Buscando portales principales…'));const candidates=await api('portal/discover','POST',{});$('candidates').replaceChildren();
  for(const c of candidates){const box=document.createElement('div');box.className='card';box.append(text('strong',c.name),text('p',c.endpoint),text('p',c.source+' · '+(c.reachable?tr('Catálogo accesible'):c.error)));
   if(c.reachable)box.append(text('p',tr('Servicios anunciados: {count}',{count:c.catalog.services.length})));
   const button=text('button',tr('Preparar confianza'));button.type='button';button.onclick=()=>{document.querySelector('[data-tab=pki]').click();const endpoint=document.getElementById('pki-peer-url'),name=document.getElementById('pki-peer-name');if(endpoint)endpoint.value=c.endpoint;if(name)name.value=c.name;};box.append(button);$('candidates').append(box);
  }message(tr('Descubrimiento terminado. Verifica y aprueba el Gateway en CA y confianza antes de seleccionar sus servicios.'));
 });
 $('trust').onclick=()=>document.querySelector('[data-tab=pki]').click();
 $('clear-logo').onclick=()=>{if(config){config.logo='';$('logo').value='';message(tr('Logo retirado del borrador. Guarda para aplicar.'))}};
 $('clear-favicon').onclick=()=>{if(config){config.favicon='';$('favicon').value='';message(tr('Favicon retirado del borrador. Guarda para aplicar.'))}};
 document.querySelector('[data-tab=portal]').addEventListener('click',safe(refresh));
})();
