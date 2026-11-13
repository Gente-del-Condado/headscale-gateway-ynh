'use strict';
const test=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const vm=require('node:vm');
const source=fs.readFileSync(__dirname+'/panel-ops.js','utf8');
const from=source.indexOf('/** @param {string} line */');
const to=source.indexOf('async function refreshLogConsole()',from);
assert.ok(from>=0 && to>from);
const context=vm.createContext({});
vm.runInContext(source.slice(from,to)+';this.classify=logClass;',context);
test('colores de registros por severidad y resultado',()=>{
 assert.equal(context.classify('[HGW] 2026/10/23 [ERROR] [HGW-TEST] fallo en proxy'),'log-error');
 assert.equal(context.classify('[HGW] [ADVERTENCIA] [HGW-TEST] configuration'),'log-warning');
 assert.equal(context.classify('[HGW-ADMIN] status=200 request_id=1'),'log-success');
 assert.equal(context.classify('[HGW-PROXY-REQ] method=GET host=example'),'log-request');
 assert.equal(context.classify('iniciando servicios'),'log-info');
});
test('renderLogLines crea nodos de texto, no HTML interpretado',()=>{
 const children=[];
 class E {constructor(){this.textContent='';this.className='';}}
 context.document={createDocumentFragment(){return{children:[],append(e){this.children.push(e)}}},createElement(){return new E()}};
 const target={replaceChildren(fragment){children.push(...fragment.children)}};
 context.renderLogLines(target,['<script>alert(1)</script>']);
 assert.equal(children.length,1);
 assert.equal(children[0].textContent,'<script>alert(1)</script>\n');
});

test('UTC se convierte a Madrid con cambio de horario y formato configurable',()=>{
 assert.match(context.formatLogTime('[HGW] 2026/07/01 19:43:00.123456 info','Europe/Madrid'),/21:43:00/);
 assert.match(context.formatLogTime('[HGW] 2026/12/01 19:43:00 info','Europe/Madrid'),/20:43:00/);
 assert.match(context.formatLogTime('[HGW] 2026/07/01 19:43:00 info','UTC'),/19:43:00/);
 assert.equal(context.formatLogTime('sin fecha','Europe/Madrid'),'sin fecha');
 assert.equal(context.formatLogTime('2026/07/01 19:43:00','raw'),'2026/07/01 19:43:00');
});

test('solo etiquetas canónicas con evento, sin inferencia por texto',()=>{
 for(const label of ['ADVERTENCIA','WARNING'])assert.equal(context.logLevel('['+label+'] [HGW-TEST] error status=500'),'warn');
 for(const text of ['WARNING: fallo','[WARN ] [HGW-TEST] aviso','[WARN] aviso','status=500'])assert.equal(context.logLevel(text),'unclassified');
 assert.equal(context.logLevel('[INFORMACIÓN] [HGW-TEST] ERROR status=500'),'info');
});

test('filtrado acumulativo usa la misma severidad en todos los niveles',()=>{
 const lines=['[ERROR] [HGW-TEST] fallo','[ADVERTENCIA] [HGW-TEST] aviso','[INFORMACIÓN] [HGW-TEST] inicio','[DEPURACIÓN] [HGW-TEST] detalle','[TRAZA] [HGW-TEST] petición'];
 for(const [i,filter] of ['error','warn','info','debug','trace'].entries())assert.equal(lines.filter(line=>context.logMatchesLevel(line,filter)).length,i+1);
 assert.equal(lines.filter(line=>context.logMatchesLevel(line,'all')).length,5);
});

test('filtro real del visor combina búsqueda, nivel y tres canales',()=>{
 const controls={'log-filter-search':{value:''},'log-filter-level':{value:'warn'},'log-timezone':{value:'raw'},'log-clock':{value:'24'},'log-follow':{checked:false}};
 const shown={};context.logInput=id=>controls[id];context.el=id=>({replaceChildren(fragment){shown[id]=fragment.children.map(c=>c.textContent).join('')}});
 const lines=['[ADVERTENCIA] [HGW-TEST] aviso uno','[WARNING] [HGW-TEST] aviso dos','[ERROR] [HGW-TEST] fallo TLS','[INFORMACIÓN] [HGW-TEST] inicio'];
 vm.runInContext(`for(const name of Object.keys(logHistory))logHistory[name]=${JSON.stringify(lines)};showVisibleLogs();`,context);
 for(const name of ['gateway','proxy','admin']){const text=shown['log-console-'+name];assert.match(text,/aviso uno/);assert.match(text,/aviso dos/);assert.match(text,/fallo TLS/);assert.doesNotMatch(text,/inicio/);}
 controls['log-filter-search'].value='tls';context.showVisibleLogs();
 for(const text of Object.values(shown)){assert.match(text,/fallo TLS/);assert.doesNotMatch(text,/aviso/);}
});
