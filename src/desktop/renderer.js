'use strict';const el=id=>document.getElementById(id);let state,messages={};const originals=new WeakMap();
function tr(s){return messages[s]||s;}
function translateTree(){
 const walker=document.createTreeWalker(document.body,NodeFilter.SHOW_TEXT);
 while(walker.nextNode()){const n=walker.currentNode;if(n.parentElement.closest('script,style,option'))continue;
 const old=originals.get(n);const source=old&&n.nodeValue===old.rendered?old.source:n.nodeValue;
 const rendered=source.replace(/^(\s*)(.*?)(\s*)$/,(m,a,b,c)=>a+tr(b)+c);originals.set(n,{source,rendered});n.nodeValue=rendered;}
 for(const n of document.querySelectorAll('[placeholder],[aria-label]'))for(const attr of ['placeholder','aria-label'])if(n.hasAttribute(attr)){
 const key='data-source-'+attr;if(!n.hasAttribute(key))n.setAttribute(key,n.getAttribute(attr));n.setAttribute(attr,tr(n.getAttribute(key)));}
}

function render(s){state=s;messages=s.messages||{};document.documentElement.lang=s.config?.language||'es';
 const select=el('language');if(s.languages&&select.options.length!==s.languages.length){select.replaceChildren();for(const code of s.languages){const o=document.createElement('option');o.value=code;o.textContent=new Intl.DisplayNames([code],{type:'language'}).of(code);select.append(o);}}select.value=s.config?.language||'es';
el('status').textContent=s.connected?'● Conectado':'○ Sin conexión';el('message').textContent=s.message;el('updated').textContent=s.updated?tr('Última consulta:')+' '+new Date(s.updated).toLocaleTimeString(document.documentElement.lang):'';el('count').textContent=s.requests.length;el('requests').replaceChildren();for(const r of s.requests){const li=document.createElement('li');li.textContent=r.name+' · '+r.route;el('requests').append(li);}if(!s.requests.length)el('requests').textContent='Sin solicitudes pendientes';el('total').textContent=s.nodes.length;nodes();translateTree();}
function nodes(){if(!state)return;const q=el('search').value.toLowerCase();el('nodes').replaceChildren();for(const n of state.nodes){const text=[n.given_name||n.name,n.user,...(n.ip_addresses||[])].join(' ');if(!text.toLowerCase().includes(q))continue;const li=document.createElement('li');li.textContent=(n.online?'● ':'○ ')+(n.given_name||n.name);const small=document.createElement('small');small.textContent=[n.user,...(n.ip_addresses||[])].join(' · ');li.append(small);el('nodes').append(li);}}
async function run(fn){el('error').textContent='';try{await fn();}catch(e){el('error').textContent=tr(e.message);translateTree();}}
el('config').onsubmit=e=>{e.preventDefault();run(async()=>{render(await gateway.configure({url:el('url').value,token:el('token').value,remember:el('remember').checked,notifications:el('notifications').checked,language:el('language').value}));el('token').value='';if(state.connected)await view(false);});};el('refresh').onclick=()=>run(async()=>render(await gateway.refresh()));el('open').onclick=()=>run(()=>gateway.open());el('disconnect').onclick=()=>run(async()=>{render(await gateway.disconnect());el('token').value='';el('remember').checked=false;});el('search').oninput=nodes;gateway.onState(render);run(async()=>{const s=await gateway.snapshot();el('url').value=s.config.url;el('remember').checked=s.config.remember;el('notifications').checked=s.config.notifications;el('storage').textContent=s.secureStorage?'Almacén seguro disponible.':'Token solo en memoria: no hay almacén seguro disponible.';render(s);if(!s.hasToken)await view(true);});

el('test-notification').onclick=()=>run(()=>gateway.testNotification());

el('language').onchange=()=>run(async()=>render(await gateway.language(el('language').value)));

async function view(preferences){el('dashboard').hidden=preferences;el('preferences-view').hidden=!preferences;await gateway.view(preferences);(preferences?el('url'):el('preferences')).focus();}
el('preferences').onclick=()=>run(()=>view(true));el('back').onclick=()=>run(()=>view(false));el('hide').onclick=()=>gateway.hide();
document.addEventListener('keydown',e=>{if(e.key==='Escape'){if(!el('preferences-view').hidden)run(()=>view(false));else gateway.hide();}});
