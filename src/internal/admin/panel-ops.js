'use strict';
// La consulta de registros, de solo lectura, funciona mientras su pestaña está visible.
let logLoading = false;
let logSettingsLoaded = false;
/** @param {string} id */
const logInput = id => /** @type {HTMLInputElement} */ (el(id));
/** @param {string} line */
function logLevel(line){
 const levels={ERROR:'error',ADVERTENCIA:'warn',INFORMACIÓN:'info',DEPURACIÓN:'debug',TRAZA:'trace',WARNING:'warn',INFORMATION:'info',DEBUG:'debug',TRACE:'trace'};
 const tag=/\[([^\]\r\n]+)\] \[HGW-[A-Z0-9-]+\]/.exec(line);
 const available=typeof window!=='undefined'&&window.gatewayLogLevels||levels;
 return tag ? (available[tag[1]]||'unclassified') : 'unclassified';
}
function logMatchesLevel(line,filter){
 if(filter==='all')return true;
 if(filter==='unclassified')return logLevel(line)==='unclassified';
 const order={error:0,warn:1,info:2,debug:3,trace:4};
 return order[logLevel(line)]<=order[filter];
}
function logClass(line){
 const level=logLevel(line);
 if(level==='error')return 'log-error';
 if(level==='warn')return 'log-warning';
 if(level==='debug'||level==='trace')return 'log-request';
 if(/\b(status=[23]\d\d|completad[oa]|correctamente|conectado|success|ok)\b/i.test(line))return 'log-success';
 if(/\b(method=(GET|POST|PUT|DELETE|PATCH)|request_id=|\[HGW-PROXY-REQ)/i.test(line))return 'log-request';
 return 'log-info';
}
const logLevelLabel=value=>({error:'Error',warn:'Advertencia',info:'Información',debug:'Depuración',trace:'Traza'}[value]||value);
// Los archivos usan UTC; el visor convierte la hora respetando los cambios estacionales.
function formatLogTime(line, zone='local', clock='24') {
 if(zone==='raw')return line;
 return line.replace(/(\d{4})\/(\d{2})\/(\d{2}) (\d{2}:\d{2}:\d{2})(?:\.(\d+))?/, (raw,y,m,d,t,fraction)=>{
  const date=new Date(`${y}-${m}-${d}T${t}.${(fraction||'0').slice(0,3).padEnd(3,'0')}Z`);
  if(Number.isNaN(date.getTime()))return raw;
  try{return new Intl.DateTimeFormat(typeof window!=='undefined'&&window.gatewayLanguage?window.gatewayLanguage():'es',{year:'numeric',month:'2-digit',day:'2-digit',hour:'2-digit',minute:'2-digit',second:'2-digit',hour12:clock==='12',timeZone:zone==='local'?undefined:zone,timeZoneName:'short'}).format(date)}catch{return raw}
 });
}
function renderLogLines(target,lines){
  const fragment=document.createDocumentFragment();
  for(const line of lines){const span=document.createElement('span');span.className='log-line '+logClass(line);span.textContent=line+'\n';fragment.append(span);}
  target.replaceChildren(fragment);
}
const logHistory={gateway:[],proxy:[],admin:[]};
function showVisibleLogs(){
 const needle=logInput('log-filter-search').value.trim().toLowerCase();
 const filter=logInput('log-filter-level').value;
 const event=logInput('log-filter-event')?.value.trim().toUpperCase()||'';
 for(const name of ['gateway','proxy','admin']){
  const lines=logHistory[name].filter(line=>{
   if(needle && !line.toLowerCase().includes(needle))return false;
   if(event && !(line.match(/\[HGW-[A-Z0-9-]+\]/g)||[]).some(tag=>tag.slice(1,-1).startsWith(event)))return false;
   return logMatchesLevel(line,filter);
  });
  const target=el('log-console-'+name);renderLogLines(target,lines.map(line=>formatLogTime(line,logInput('log-timezone').value,logInput('log-clock').value)));if(logInput('log-follow').checked)target.scrollTop=target.scrollHeight;
 }
}
async function refreshLogConsole(){
  if(logLoading || el('app').hidden || (/** @type {HTMLElement} */ (document.querySelector('[data-page="logs"]'))).hidden)return;
  logLoading = true;
  try {
    const channels=['gateway','proxy','admin'];
    const all=await Promise.all([...channels.map(name=>api('diagnostics/logs/'+name+'?limit=100')),api('diagnostics/logs')]);
    for(let i=0;i<channels.length;i++){
      const consoleEl=el('log-console-'+channels[i]);
      logHistory[channels[i]]=all[i].lines||[];
    }
    showVisibleLogs();
    const info=all[3];
    el('log-status').textContent=window.tr("3 archivos · últimas 100 líneas de cada uno · {p0}", {p0:(new Date().toLocaleTimeString())});
    el('log-debug-status').textContent=`Nivel efectivo: ${logLevelLabel(info.effective_level||'info')}${info.debug_until?window.tr(" · depuración temporal hasta {p0}", {p0:(info.debug_until)}):''}`;
    if(!logSettingsLoaded){logInput('log-dir').value=info.directory;logInput('log-max-mb').value=info.max_mb;logInput('log-backups').value=info.backups;logInput('log-level').value=info.level||'info';logSettingsLoaded=true;}
  }catch(err){el('log-status').textContent=err.message;}finally{logLoading=false;}
}
logInput('log-filter-level').addEventListener('change',showVisibleLogs);
logInput('log-filter-search').addEventListener('input',showVisibleLogs);
logInput('log-filter-event').addEventListener('input',showVisibleLogs);
el('log-refresh').addEventListener('click',()=>run(refreshLogConsole));
el('log-copy').addEventListener('click',()=>{
  const value=['gateway','proxy','admin'].map(name=>'=== '+name+'.log ===\n'+el('log-console-'+name).textContent).join('\n\n');
  if(navigator.clipboard?.writeText){navigator.clipboard.writeText(value).then(()=>notice('Líneas copiadas'),err=>notice(err.message,true));}
  else notice('Portapapeles no disponible en este navegador',true);
});
document.querySelector('[data-tab="logs"]').addEventListener('click',()=>run(refreshLogConsole));
setInterval(()=>{if(logInput('log-follow').checked)void refreshLogConsole();},3000);
el('log-settings-form').addEventListener('submit',event=>{event.preventDefault();run(async()=>{
  const data={directory:logInput('log-dir').value.trim(),max_mb:Number(logInput('log-max-mb').value),backups:Number(logInput('log-backups').value),level:logInput('log-level').value};
  if(!confirm('¿Aplicar la nueva ubicación y rotación de logs?'))return;
  await api('diagnostics/logs/settings','PUT',data);
  await refreshLogConsole();notice('Nivel y rotación de registros actualizados');
});});
el('log-debug-start').addEventListener('click',()=>run(async()=>{
 const minutes=Number(logInput('log-debug-duration').value);
 if(!confirm(window.tr("¿Activar depuración durante {p0} minutos? Los registros pueden contener datos de infraestructura.", {p0:(minutes)})))return;
 await api('diagnostics/logs/debug','POST',{minutes});await refreshLogConsole();notice('Depuración temporal activado');
}));
el('log-debug-stop').addEventListener('click',()=>run(async()=>{
 await api('diagnostics/logs/debug','POST',{minutes:0});await refreshLogConsole();notice('Depuración temporal desactivado');
}));
el('proxy-clear-idle').addEventListener('click',()=>run(async()=>{
  await api('diagnostics/proxy/clear-idle','POST',{});
  notice('Conexiones inactivas del proxy cerradas; no hay caché de respuestas HTTP');
}));

for(const id of ['log-timezone','log-clock']){
 try{const saved=localStorage.getItem('hgw.'+id);if(saved)logInput(id).value=saved;}catch{}
 logInput(id).addEventListener('change',()=>{try{localStorage.setItem('hgw.'+id,logInput(id).value)}catch{}showVisibleLogs()});
}
el('log-clear').addEventListener('click',()=>run(async()=>{
 if(!confirm('¿Borrar los tres archivos de logs y sus copias rotadas? La auditoría SQLite y el journal del SO se conservan.'))return;
 await api('diagnostics/logs/clear','POST',{confirm:'LIMPIAR'});
 for(const name of Object.keys(logHistory))logHistory[name]=[];
 showVisibleLogs();await refreshLogConsole();notice('Logs del Gateway limpiados');
}));
