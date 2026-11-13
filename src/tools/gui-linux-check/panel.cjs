'use strict';
// Regresión del renderizador completo del panel, aislada y con respuestas de API simuladas.
const {app,BrowserWindow}=require('electron');
const fs=require('node:fs'),path=require('node:path'),http=require('node:http'),assert=require('node:assert/strict');
const root=path.resolve(__dirname,'../..');
let server;
app.whenReady().then(async()=>{try{
 server=http.createServer((req,res)=>{
  if(req.url==='/admin/locales'){res.setHeader('Content-Type','application/json');return res.end(JSON.stringify(['es','en','fr']));}
  if(req.url==='/admin/locales/levels'){res.setHeader('Content-Type','application/json');return res.end(JSON.stringify({ERROR:'error',ADVERTENCIA:'warn',INFORMACIÓN:'info',DEPURACIÓN:'debug',TRAZA:'trace',WARNING:'warn',INFORMATION:'info',DEBUG:'debug',TRACE:'trace'}));}
  if(req.url.startsWith('/admin/locales/')){res.setHeader('Content-Type','application/json');const lang=path.basename(req.url);return res.end(lang==='fr.json'?JSON.stringify({'Usuarios y claves':'Utilisateurs et clés'}):fs.readFileSync(path.join(root,'internal/i18n/locales',lang)));}
  if(req.url.startsWith('/api/')){res.setHeader('Content-Type','application/json');return res.end(JSON.stringify(req.url.endsWith('/methods')?{mode:'token'}:{authenticated:false}));}
  const name=req.url==='/admin/'?'panel.html':path.basename(req.url);
  const file=path.join(root,'internal/admin',name);
  if(!fs.existsSync(file)){res.writeHead(404);return res.end();}
  res.setHeader('Content-Type',name.endsWith('.js')?'application/javascript':name.endsWith('.css')?'text/css':'text/html');res.end(fs.readFileSync(file));
 });
 await new Promise(r=>server.listen(0,'127.0.0.1',r));
 const win=new BrowserWindow({width:1200,height:900,show:true,webPreferences:{sandbox:true,contextIsolation:true,nodeIntegration:false,backgroundThrottling:false}});
 await win.loadURL(`http://127.0.0.1:${server.address().port}/admin/`);
 await win.webContents.executeJavaScript(`renderStatus({setup_required:false,subscription:{state:'disabled',enabled:false,allowed:true}});if(!document.getElementById('status').textContent.includes('Módulo desactivado'))throw Error('subscription status rendering');`);
 const results=await win.webContents.executeJavaScript(`(async()=>{
  const ok=(v,m)=>{if(!v)throw Error(m);};const tick=()=>new Promise(r=>setTimeout(r,200));
  const results=[];window.prompt=()=>{throw Error('prompt unsupported');};window.confirm=()=>true;
  el('app').hidden=false;el('login').hidden=true;currentRole='admin';
  window.fixtureNode={id:1,name:'lab',available_routes:['192.0.2.0/24','198.51.100.0/24'],approved_routes:['192.0.2.0/24','198.51.100.0/24']};
  window.writes=[];window.refreshes=0;
  api=async(p,m,b)=>{
   if(p==='network/nodes')return {nodes:[JSON.parse(JSON.stringify(window.fixtureNode))]};
   if(m==='POST'){window.writes.push({p,b});return {local_removed:true,uninstalled:false,warning:'Sin permisos',instructions:'Retirar manualmente'};}
   if(p==='pki')return {local_ca:[{name:'lab-ca',fingerprint:'a'.repeat(64),expires:'2030-01-01'}]};
   if(p==='network/admin/policy')return {policy:'{"grants":[]}',revision:'fixture-rev'};
   if(p==='network/admin/policy/templates')return [{id:'fixture-template',name:'Ejemplo',policy:'{"grants":[{"src":["*"],"dst":["*"],"ip":["tcp:443"]}]}',source:'builtin'}];
   return {};
  };
  refreshNetwork=async()=>{window.refreshes++;};
  selectedNode=window.fixtureNode;el('headscale-node-routes').click();await tick();
  let d=document.querySelector('dialog');ok(d,'route dialog');d.querySelector('button[type=button]').click();await tick();ok(!window.writes.length,'cancel must not write');
  el('headscale-node-routes').click();await tick();d=document.querySelector('dialog');d.querySelector('input[type=checkbox]').checked=false;d.querySelector('form').requestSubmit();await tick();
  ok(window.writes.length,'missing route write: '+el('notice').textContent+' role='+currentRole+' dialogs='+[...document.querySelectorAll('dialog')].map(d=>d.outerHTML).join(''));ok(JSON.stringify(window.writes[0].b.routes)==='["198.51.100.0/24"]','revoke one, preserve another');
  window.writes=[];el('headscale-node-routes').click();await tick();d=document.querySelector('dialog');d.querySelectorAll('input[type=checkbox]').forEach(c=>c.checked=false);d.querySelector('form').requestSubmit();await tick();ok(window.writes[0].b.routes.length===0,'revoke all');
  window.writes=[];el('headscale-node-routes').click();await tick();d=document.querySelector('dialog');window.fixtureNode.approved_routes=[];d.querySelector('form').requestSubmit();await tick();ok(!window.writes.length,'stale inventory must not write');ok(el('notice').textContent.includes('han cambiado'),'stale message');results.push('routes: cancel, revoke one/all, stale inventory');
  document.querySelector('[data-tab=pki]').click();await tick();
  [...el('pki-local-rows').querySelectorAll('button')].find(b=>b.textContent==='Eliminar del Gateway y SO').click();await tick();
  d=document.querySelector('dialog');const submit=d.querySelector('button[type=submit]'),field=d.querySelector('input');ok(submit.disabled,'CA requires fingerprint');field.value='wrong';field.dispatchEvent(new Event('input'));ok(submit.disabled,'wrong fingerprint blocked');field.value='a'.repeat(64);field.dispatchEvent(new Event('input'));ok(!submit.disabled,'correct fingerprint enabled');d.querySelector('form').requestSubmit();await tick();ok(window.writes.length===1&&window.writes[0].p.endsWith('/uninstall'),'CA removal request');ok(el('pki-message').textContent.includes('CA eliminada')&&el('pki-message').textContent.includes('Sin permisos'),'local success separate from OS failure');results.push('CA: no prompt, exact fingerprint, local removal with OS warning');
  ok(el('headscale-management').closest('[data-page]').dataset.page==='headscale-users','users separate');ok(el('hs-tpl-select').closest('[data-page]').dataset.page==='policy-templates','templates separate');
  document.querySelector('[data-tab=policy-templates]').click();await tick();el('hs-tpl-use').click();await tick();ok(!document.querySelector('[data-page=policy-dns]').hidden,'template navigates to editor');ok(el('hs-policy-editor').value.includes('tcp:443'),'draft not overwritten');ok(!el('hs-policy-editor').disabled,'editor enabled');results.push('navigation and template draft preserved');
  await refreshBackupOverview({items:[{id:'old',created_at:'2025-01-01',bytes:1},{id:'latest',created_at:'2026-10-25',bytes:1048576}],settings:{interval_hours:24,keep:7},encryption_ready:true});ok(el('overview-backups-summary').textContent.includes('latest'),'latest backup by timestamp');ok(el('overview-backups-schedule').textContent.includes('24'),'backup schedule');await refreshBackupOverview({items:[],settings:{}});ok(el('overview-backups-summary').textContent.includes('Todavía no'),'empty backups');results.push('backup summary: latest, schedule, empty');
  const baseApi=api;window.connectionState='Running';
  api=async(p,m,b)=>{
   if(p==='network/info')return {enabled:true,gateway_vpn_state:window.connectionState,manually_paused:window.connectionState==='Stopped',gateway_preferences:{}};
   if(p==='network/disconnect'){window.writes.push({p,b});window.connectionState='Stopped';return {state:'Stopped'};}
   if(p==='network/login'){window.connectionState='Running';return {state:'Running'};}
   if(p==='network/ping')return {mode:b.mode,error:'connection refused',port:'22'};
   if(p==='backups')return {encryption_ready:true,settings:{},items:[{id:'backup-fixture',created_at:'2026-10-25T02:53:02Z',components:{full:true},acl:'included',magic_dns:'generated_export',bytes:555,sha256:'f'.repeat(64)}]};
   if(p==='network/admin/users')return {users:[{id:1,name:'usuario-con-un-nombre-muy-largo@ejemplo.invalid'}]};
   if(p==='network/admin/keys')return {preAuthKeys:[{id:1,user:{name:'usuario-con-un-nombre-muy-largo@ejemplo.invalid'},expiration:'2027-01-01'}]};
   return baseApi(p,m,b);
  };
  await initializeNetwork();ok(el('network-login').textContent.includes('Pausar')&&el('network-login').classList.contains('danger'),'running red pause');
  window.writes=[];el('network-login').click();await tick();document.querySelector('dialog button[type=button]').click();await tick();ok(!window.writes.length,'disconnect cancelled');
  el('network-login').click();await tick();document.querySelector('dialog form').requestSubmit();await tick();ok(window.writes[0].p==='network/disconnect','disconnect endpoint');ok(el('network-login').textContent.includes('Reconectar')&&!el('network-login').classList.contains('danger'),'stopped reconnect');
  el('network-login').click();await tick();ok(el('network-login').classList.contains('danger'),'reconnect');results.push('connection button: state, cancel, disconnect, reconnect');
  const beforeGuard=api;api=async(p,m,b)=>p==='network/info'?{...(await beforeGuard(p,m,b)),pause_allowed:false,pause_block_reason:'Acceso por VPN: utiliza el panel local'}:beforeGuard(p,m,b);
  window.writes=[];await initializeNetwork();ok(el('network-login').disabled,'VPN pause button blocked');ok(el('network-login-status').textContent.includes('panel local'),'recovery instructions visible');
  el('network-login').disabled=false;el('network-login').click();await tick();ok(!window.writes.length,'stale UI must not send pause');ok(el('network-login').disabled,'guard remains disabled');
  api=beforeGuard;await initializeNetwork();ok(!el('network-login').disabled,'LAN pause remains available');

  selectedNode=window.fixtureNode;await runNodeDiagnostics(['tcp-22']);ok(el('network-diagnostic-summary').textContent.includes('Conexión rechazada'),'plain diagnostic');ok(el('network-ping-result').textContent.includes('connection refused'),'raw preserved');document.querySelector('[data-diagnostic-tab=technical]').click();ok(el('network-diagnostic-summary').hidden&&!el('network-ping-result').hidden,'technical tab');document.querySelector('[data-diagnostic-tab=summary]').click();ok(!el('network-diagnostic-summary').hidden,'summary tab');ok(diagnosticExplanation('tls-443',{error:'x509',connected:true}).title.includes('TLS'),'TLS explanation');results.push('diagnostics: plain explanation, raw result, tab switching');
  document.querySelector('[data-tab=backups]').click();await tick();ok(!el('backup-rows').textContent.includes('included')&&!el('backup-rows').textContent.includes('generated_export'),'backup states translated');ok(el('backup-rows').textContent.includes('555 B'),'small backup size');ok(el('backup-rows').querySelector('.backup-actions button'),'separate actions');results.push('backup contents: readable states, bytes, buttons');
  document.querySelector('[data-tab=headscale-users]').click();await tick();
  return results;
 })()`);
 results.forEach(r=>console.log('PASS '+r));assert.equal(results.length,7);
 for(const width of [1280,1040,850,600]){
  win.setSize(width,900);await new Promise(r=>setTimeout(r,150));
  const geometry=await win.webContents.executeJavaScript(`(()=>{const cols=[...document.querySelectorAll('#headscale-management>.grid>div')].map(e=>{const r=e.getBoundingClientRect();return {left:r.left,right:r.right,top:r.top,bottom:r.bottom}});return {cols,overflow:document.documentElement.scrollWidth>innerWidth+2}})()`);
  const [a,b]=geometry.cols;assert.ok(a.right<=b.left+1||a.bottom<=b.top+1,'columns overlap at '+width);assert.equal(geometry.overflow,false,'page overflows at '+width);
  if(width===1040)fs.writeFileSync(path.join(root,'../docs/capturas/usuarios-responsive.png'),(await win.webContents.capturePage()).toPNG());
 }
 console.log('PASS users responsive layout: 1280,1040,850,600');
 win.setSize(1280,1000);await new Promise(r=>setTimeout(r,150));
 await win.webContents.executeJavaScript(`(async()=>{
  const tick=()=>new Promise(r=>setTimeout(r,200));const ok=(v,m)=>{if(!v)throw Error(m)};
  const config={enabled:true,title:'Lab Portal',description:'Services',accent:'#38bdf8',theme:'dark',logo:'',favicon:'',search:true,show_admin:true,publish:false,federation:true,discovery:true,local:{app:{label:'Example',hidden:false,share:false}},selected:[]};
  const response={config,local_names:['app'],url:'https://lab.example/',peers:[{peer:'a'.repeat(64),name:'Other Gateway',endpoint:'https://other.example',catalog:{services:[{id:'remote',name:'Remote App',url:'https://remote.other.example'}]}}]};
  let saved=null;
  api=async(p,m,b)=>{if(p==='portal'&&m==='PUT'){saved=b;response.config=b;return {saved:true}}if(p==='portal'||p==='portal/sync')return response;if(p==='portal/discover')return [{name:'Candidate',endpoint:'https://candidate.example',source:'fixture',reachable:true,catalog:{services:[]}}];return {}};
  document.querySelector('[data-tab=portal]').click();await tick();
  ok(document.getElementById('portal-title').value==='Lab Portal','portal loaded');
  document.getElementById('portal-title').value='Unsaved title';
  document.querySelector('#portal-peers input').checked=true;
  document.getElementById('portal-sync').click();await tick();
  ok(document.getElementById('portal-title').value==='Unsaved title','sync discarded draft');
  ok(document.querySelector('#portal-peers input').checked,'sync discarded remote selection');
  document.querySelector('#portal-local input[data-key=share]').checked=true;
  document.getElementById('portal-form').requestSubmit();await tick();
  ok(saved.title==='Unsaved title'&&saved.local.app.share&&saved.selected.length===1,'portal save');
  document.getElementById('portal-discover').click();await tick();
  ok(document.getElementById('portal-candidates').textContent.includes('Candidate'),'discovery results');
 })()`);
 await win.webContents.executeJavaScript("document.getElementById('toast-stack').replaceChildren();window.scrollTo(0,0)");
 fs.writeFileSync(path.join(root,'../docs/capturas/portal-administracion.png'),(await win.webContents.capturePage()).toPNG());
 console.log('PASS portal: settings, sharing, remote selection, draft preservation, discovery');

 await win.webContents.executeJavaScript(`(async()=>{
 const tick=()=>new Promise(r=>setTimeout(r,200));let writes=[];
 api=async(p,m,b)=>{if(p==='pki/discover')return {candidates:[{name:'Windows',endpoint:'https://windows.example',targets:[{network:'Tailnet',ip:'100.64.0.13'},{network:'LAN',ip:'192.168.1.71'}]}]};if(p==='pki/discover/download'){writes.push({p,m,b});return [{endpoint:'https://windows.example',anchor:{fingerprint:'f'.repeat(64),approved:false}}]};if(p==='pki')return {peers:[{name:'Windows',endpoint:'https://windows.example',fingerprint:'f'.repeat(64),approved:false}]};return {}};
 document.querySelector('[data-tab=pki]').click();await tick();document.getElementById('pki-lan-discover').click();await tick();
 const list=document.getElementById('pki-discovery');if(!list.textContent.includes('Tailnet 100.64.0.13'))throw Error('missing network plan');
 list.querySelector('button').click();await tick();
 if(writes.length!==1||writes[0].p!=='pki/discover/download'||writes[0].m!=='POST')throw Error('download did not use protected operation');
 if(!document.getElementById('pki-peer-rows').textContent.includes('Pendiente de verificar'))throw Error('download not pending');
 if(!document.getElementById('pki-peer-rows').textContent.includes('Verificar y aprobar'))throw Error('approval missing');
 if(writes.some(w=>w.p.endsWith('/approve')))throw Error('automatic approval');
 })()`);
 console.log('PASS CA discovery: Tailnet/LAN plan, protected download, pending state, explicit approval only');

 await win.webContents.executeJavaScript('window.referenceFixture='+fs.readFileSync(process.env.HGW_REFERENCE_FIXTURE||'/tmp/hgw-reference-catalog.json','utf8'));
 await win.webContents.executeJavaScript(`(async()=>{
 const tick=()=>new Promise(r=>setTimeout(r,150)),ok=(v,m)=>{if(!v)throw Error(m)},e=id=>document.getElementById('ref-'+id);
 api=async(p)=>p==='reference'?window.referenceFixture:p==='portal'?{peers:[{name:'Lab remote',endpoint:'https://remote.test',peer:'a'.repeat(64)}]}:{};
 document.querySelector('[data-tab="api-reference"]').click();await tick();
 ok(e('index').querySelectorAll('button').length>90,'incomplete API index');
 e('search').value='portal';e('search').dispatchEvent(new Event('input'));ok(e('index').querySelectorAll('button').length===4,'portal filtering');
 e('search').value='';e('search').dispatchEvent(new Event('input'));
 const choose=(label)=>{const b=[...e('index').querySelectorAll('button')].find(b=>b.textContent===label);ok(b,'missing '+label);b.click()};
 choose('GET /api/v1/status');
 const originalFetch=window.fetch;window.referenceRequests=[];
 window.fetch=async(url,options)=>{window.referenceRequests.push({url,options});return new Response(JSON.stringify({ok:true}),{status:200,headers:{'content-type':'application/json'}})};
 e('form').requestSubmit();await tick();ok(window.referenceRequests.length===1&&e('status').textContent.includes('HTTP 200'),'GET probe');
 choose('POST /api/v1/reload');e('form').requestSubmit();await tick();ok(window.referenceRequests.length===1,'write without confirmation');
 e('confirm').checked=true;e('form').requestSubmit();await tick();ok(window.referenceRequests.length===2&&!e('confirm').checked,'confirmed write once');
 e('mode').value='manual';e('mode').dispatchEvent(new Event('change'));e('path').value='https://attacker.test';e('form').requestSubmit();await tick();ok(window.referenceRequests.length===2,'external URL accepted');
 choose('GET /api/v1/status');e('target').value='a'.repeat(64);e('target').dispatchEvent(new Event('change'));e('form').requestSubmit();await tick();ok(window.referenceRequests.at(-1).url==='/api/v1/reference/peer/'+'a'.repeat(64),'remote path');ok(e('path').value==='/.well-known/headscale-gateway','remote catalog only');
 e('token').value='test-only';e('clear').click();ok(e('token').value===''&&e('output').textContent==='','clear credentials');
 window.fetch=originalFetch;e('kind').value='cli';e('kind').dispatchEvent(new Event('change'));ok(e('index').textContent.includes('doctor'),'CLI missing');e('kind').value='external';e('kind').dispatchEvent(new Event('change'));ok(e('index').textContent.includes('Technitium'),'integration missing');e('kind').value='api';e('kind').dispatchEvent(new Event('change'));choose('GET /api/v1/network/nodes');
 })()`);
 for(const width of [1200,850,600]){win.setSize(width,1000);await new Promise(r=>setTimeout(r,100));assert(await win.webContents.executeJavaScript('document.documentElement.scrollWidth<=window.innerWidth+1'),'reference overflow '+width)}
 win.setSize(1200,1000);await win.webContents.executeJavaScript('window.scrollTo(0,0)');
 fs.writeFileSync(path.join(root,'../docs/capturas/api-cli.png'),(await win.webContents.capturePage()).toPNG());
 console.log('PASS API/CLI: index, filters, examples, local and federated probe, write confirmation, URL restrictions, clearing secrets, responsive layout');

 await win.webContents.executeJavaScript(`(async()=>{
 const select=document.getElementById('ui-language');select.value='en';select.dispatchEvent(new Event('change'));
 await new Promise(r=>setTimeout(r,250));
 if(document.documentElement.lang!=='en')throw Error('English not applied');
 if(!document.querySelector('[data-tab=headscale-users]').textContent.includes('Users and keys'))throw Error('English navigation');
 const box=document.createElement('p');box.id='translation-test';box.textContent=window.tr('Página {p0} de {p1} · {p2} eventos',{p0:2,p1:4,p2:8});document.body.append(box);
 if(!box.textContent.includes('Page 2 of 4'))throw Error('parameter translation');
 select.value='es';select.dispatchEvent(new Event('change'));await new Promise(r=>setTimeout(r,250));
 if(!box.textContent.includes('Página 2 de 4'))throw Error('dynamic language switch');
 localStorage.setItem('headscale-gateway-language','fr');
 })()`);
 await win.reload();await new Promise(r=>setTimeout(r,500));
 assert.equal(await win.webContents.executeJavaScript("document.documentElement.lang"),'fr');
 assert.match(await win.webContents.executeJavaScript("document.querySelector('[data-tab=headscale-users]').textContent"),/Utilisateurs/);
 console.log('PASS languages: English, dynamic parameters, switch back, restored third language');
 win.destroy();server.close();app.exit(0);
 }catch(e){console.error(e);if(server)server.close();app.exit(1);}});
