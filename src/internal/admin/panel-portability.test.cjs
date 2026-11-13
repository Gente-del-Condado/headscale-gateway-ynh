'use strict';
const test=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const vm=require('node:vm');
const source=fs.readFileSync(__dirname+'/panel-portability.js','utf8');

function setup() {
 const controls=new Map();const calls=[];const downloads=[];
 function element(id){
  if(!controls.has(id))controls.set(id,{id,value:'',checked:false,files:[],textContent:'',style:{},listeners:{},addEventListener(name,fn){this.listeners[name]=fn},replaceChildren(){this.textContent=''}});
  return controls.get(id);
 }
 const document={getElementById:element,querySelector:selector=>element(selector),createElement(tag){return {download:'',href:'',click(){downloads.push(this.download)}}}};
 const context={document,window:{tr:(s,p={})=>s.replace(/\{(\w+)\}/g,(m,k)=>Object.hasOwn(p,k)?String(p[k]):m),markRestartRequired(){calls.push(['restart'])}},api:async(...args)=>{
  calls.push(args);
  if(args[0]==='config/transfer/export')return {filename:'gateway.hgwcfg',data:'QUJDRA=='};
  if(args[0]==='config/transfer/import')return {imported:3};
  return {listener:'0.0.0.0:8443',status:'sin_regla',note:'sin permiso',instructions:['sudo comando']};
 },confirm:()=>true,Blob,Uint8Array,URL:{createObjectURL:()=>"blob:test",revokeObjectURL(){}},setTimeout:()=>{},atob,btoa,Error};
 vm.runInNewContext(source,context,{filename:'panel-portability.js'});
 return {element,calls,downloads};
}
test('los botones de exportación, importación y firewall tienen acción',()=>{
 const {element}=setup();
 for(const id of ['config-portable-export','config-portable-import','firewall-refresh'])assert.equal(typeof element(id).listeners.click,'function',id);
});
test('exportación requiere contraseña y respeta elección de credenciales',async()=>{
 const {element,calls,downloads}=setup();
 element('config-portable-password').value='contraseña-de-prueba-muy-larga';
 element('config-portable-secrets').checked=true;
 await element('config-portable-export').listeners.click();
 assert.equal(calls[0][0],'config/transfer/export');
 assert.equal(calls[0][2].secrets,true);
 assert.deepEqual(downloads,['gateway.hgwcfg']);
});
test('importación envía archivo cifrado y marca reinicio',async()=>{
 const {element,calls}=setup();
 element('config-portable-password').value='contraseña-de-prueba-muy-larga';
 element('config-portable-file').files=[{size:3,arrayBuffer:async()=>Uint8Array.of(1,2,3).buffer}];
 await element('config-portable-import').listeners.click();
 assert.equal(calls[0][0],'config/transfer/import');
 assert.equal(calls[0][2].data,'AQID');
 assert.ok(calls.some(x=>x[0]==='restart'));
});
