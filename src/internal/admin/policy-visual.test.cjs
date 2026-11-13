'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');
const path = require('node:path');

class Element {
  constructor() {this.listeners={};this.value='';this.textContent='';this.hidden=true;this.disabled=false;this.checked=false;this.children=[];this.style={};}
  addEventListener(type,callback){this.listeners[type]=callback;}
  click(){return this.listeners.click?.({target:this});}
  replaceChildren(...nodes){this.children=[...nodes];}
  append(...nodes){this.children.push(...nodes);}
  dispatchEvent(evt){return this.listeners[evt.type]?.(evt);}
  remove(){}
}
function setup(fixtures={}) {
  const els=new Map();const get=id=>{if(!els.has(id))els.set(id,new Element());return els.get(id);};
  const listeners={}; const writes=[];
  const document={
    getElementById:get,createElement:()=>new Element(),
    querySelector:()=>get('policy-nav'),
    body:new Element(),
    addEventListener:(event,fn)=>{listeners[event]=fn;},
    dispatchEvent:evt=>listeners[evt.type]?.(evt),
  };
  const fakeAPI=async (path,method='GET',body)=>{
    if(method==='PUT' && path==='network/admin/policy')writes.push(body);
    if(path.endsWith('/parse')) return JSON.parse(body.policy);
    if(path.endsWith('/templates')&&method==='GET')return [];
    if(path.endsWith('/repository')&&method==='GET')return {url:'',allow_private:false};
    if(path==='network/admin/users')return {users:fixtures.users||[]};
    if(path==='network/nodes')return {nodes:fixtures.nodes||[]};
    return {};
  };
  const context={document,window:{tr:s=>s},currentRole:'admin',api:fakeAPI,
    run:async fn=>fn(),notice:()=>{},confirm:()=>true,prompt:()=>null,
    Event:class {constructor(type){this.type=type}},
    CustomEvent:class {constructor(type,obj){this.type=type;this.detail=obj?.detail}},
    Blob:class{},URL:{createObjectURL:()=>'',revokeObjectURL:()=>{}},
    setTimeout:()=>{},navigator:{clipboard:{writeText:async()=>{}}},console};
  vm.runInNewContext(fs.readFileSync(path.join(__dirname,'panel-policy-visual.js'),'utf8'),context);
  return {get,document,writes};
}

test('editor visual conserva secciones SSH desconocidas y no escribe en Headscale',async()=>{
  const {get,writes}=setup();
  get('hs-policy-editor').value='{"ssh":[{"action":"accept","src":["*"],"dst":["autogroup:self"],"users":["root"]}],"groups":{}}';
  await get('hs-policy-mode-visual').click();
  get('hs-group-name').value='familia';get('hs-group-members').value='alice@example.org,bob@example.org';
  await get('hs-group-add').click();
  const draft=JSON.parse(get('hs-policy-editor').value);
  assert.equal(draft.groups['group:familia'].length,2);
  assert.equal(draft.ssh[0].action,'accept');
  assert.equal(writes.length,0);
});

test('editor visual sustituye grant solo cuando se pulsa añadir permiso',async()=>{
  const {get}=setup();
  get('hs-policy-editor').value='{"grants":[{"src":["group:familia"],"dst":["tag:web"],"ip":["443"]}]}';
  await get('hs-policy-mode-visual').click();
  const grant=get('hs-visual-grants').children[0];
  grant.children[1].children[0].click();
  assert.equal(JSON.parse(get('hs-policy-editor').value).grants.length,1);
  get('hs-grant-ip').value='80,443';
  await get('hs-grant-add').click();
  const result=JSON.parse(get('hs-policy-editor').value);
  assert.equal(result.grants.length,1);
  assert.equal(result.grants[0].ip[0],'80');
});

test('usuarios y nodos persisten tras cambios visuales y se normaliza usuario Headscale',async()=>{
 const {get}=setup({users:[{name:'alice'}],nodes:[{name:'nas',ip_addresses:['100.64.0.8'],tags:['tag:nas']}]});
 get('hs-policy-editor').value='{"grants":[]}';await get('hs-policy-mode-visual').click();
 assert.equal(get('hs-policy-users').children[0].value,'alice@');
 get('hs-group-name').value='admins';get('hs-group-members').value='alice@';await get('hs-group-add').click();
 assert.equal(get('hs-policy-users').children[0].value,'alice@');
 assert.ok(get('hs-policy-identities').children.some(o=>o.value==='100.64.0.8'));
});
test('editar permiso conserva via y srcPosture, valida rangos y deshace cambios',async()=>{
 const {get}=setup();get('hs-policy-editor').value=JSON.stringify({grants:[{src:['alice@'],dst:['tag:web'],ip:['tcp:443'],via:['tag:router'],srcPosture:['posture:managed']}]});
 await get('hs-policy-mode-visual').click();get('hs-visual-grants').children[0].children[1].children[0].click();
 get('hs-grant-ip').value='tcp:8000-9000,udp:53';await get('hs-grant-add').click();
 let draft=JSON.parse(get('hs-policy-editor').value);assert.deepEqual(draft.grants[0].via,['tag:router']);assert.deepEqual(draft.grants[0].srcPosture,['posture:managed']);
 await get('hs-policy-undo').click();draft=JSON.parse(get('hs-policy-editor').value);assert.deepEqual(draft.grants[0].ip,['tcp:443']);
 for(const ports of ['65536','tcp:9000-8000','443/tcp','udp:0']){get('hs-grant-src').value='alice@';get('hs-grant-dst').value='tag:web';get('hs-grant-ip').value=ports;await assert.rejects(get('hs-grant-add').click());}
});
