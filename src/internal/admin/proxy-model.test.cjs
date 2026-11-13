'use strict';
const {test} = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');
const script = fs.readFileSync(__dirname + '/proxy-model.js', 'utf8');
const context = vm.createContext({});
vm.runInContext(script + '\nglobalThis.model = GatewayProxyModel;', context);
const model = context.model;
const fields = [
  {key:'preserve_host',kind:'boolean'},
  {key:'timeout',kind:'duration'},
  {key:'max_connections',kind:'integer'},
  {key:'tls.insecure_skip_verify',kind:'boolean'},
  {key:'tls.server_name',kind:'text'},
  {key:'redirect_to',kind:'url'},
  {key:'redirect_status',kind:'select',options:['0','301','302','307','308'].map(x=>({value:x}))}
];
const asJSON = v => JSON.parse(JSON.stringify(v));
test('lectura de claves anidadas y copia inmutable', () => {
  const base={tls:{server_name:'local'}};
  const copied=model.clone(base);
  copied.tls.server_name='otro';
  assert.equal(base.tls.server_name,'local');
  assert.equal(model.pathGet(base,'tls.server_name'),'local');
});
test('false, cero y 0s se conservan y permanecen tipados', () => {
  const result=model.encode({},fields,{'preserve_host':'false','timeout':'0s','max_connections':'0','tls.insecure_skip_verify':'false','redirect_status':'0'},{});
  assert.deepEqual(asJSON(result),{preserve_host:false,timeout:'0s',max_connections:0,tls:{insecure_skip_verify:false},redirect_status:0});
});
test('borrar un valor lo devuelve a la plantilla; elimina tls vacío', () => {
  const base={preserve_host:true,tls:{insecure_skip_verify:false},max_connections:7};
  const result=model.encode(base,fields,{'preserve_host':'','tls.insecure_skip_verify':'','max_connections':''},{});
  assert.deepEqual(asJSON(result),{});
  assert.deepEqual(base,{preserve_host:true,tls:{insecure_skip_verify:false},max_connections:7});
});
test('no se pierde configuración de proxy desconocida al modificar campos conocidos', () => {
  const original={future_option:{enabled:true},tls:{future_flag:'abc',server_name:'old'},request_headers:{'X-Original':'yes'}};
  const result=model.encode(original,fields,{'preserve_host':'true','tls.server_name':'new'},{});
  assert.deepEqual(asJSON(result),{future_option:{enabled:true},preserve_host:true,tls:{future_flag:'abc',server_name:'new'},request_headers:{'X-Original':'yes'}});
});
test('las cabeceras se sustituyen, las del otro grupo se conservan', () => {
  const initial={response_headers:{'X-Safe':'ok'},request_headers:{'X-Remove':'yes'}};
  const result=model.encode(initial,fields,{}, {request_headers:{'X-New':'ok'}});
  assert.deepEqual(asJSON(result),{response_headers:{'X-Safe':'ok'},request_headers:{'X-New':'ok'}});
});
test('rechaza valores inválidos y conserva opciones existentes sin forzarlas', () => {
  for(const vals of [{'max_connections':'-1'},{'timeout':'3dias'},{'redirect_status':'303'},{'preserve_host':'yes'}]) {
    assert.throws(()=>model.encode({},fields,vals,{}));
  }
  const result=model.encode({redirect_status:308},fields,{},{});
  assert.equal(result.redirect_status,308);
});
test('valores numéricos de redirección solo cuando declarados', () => {
  assert.equal(model.encode({},fields,{redirect_status:'301'},{}).redirect_status,301);
  assert.deepEqual(asJSON(model.encode({redirect_status:301},fields,{redirect_status:''},{})),{});
});

test('egress_mode de servicio: herencia, cadenas admitidas y rechazo de valores no implementados',()=>{
  const extra={key:'egress_mode',kind:'choice',options:[{value:'system'},{value:'tailnet'}]};
  const defs=[extra];
  assert.deepEqual(asJSON(model.encode({},defs,{egress_mode:'tailnet'},{})),{egress_mode:'tailnet'});
  assert.deepEqual(asJSON(model.encode({egress_mode:'tailnet'},defs,{egress_mode:'system'},{})),{egress_mode:'system'});
  assert.deepEqual(asJSON(model.encode({egress_mode:'tailnet'},defs,{egress_mode:''},{})),{});
  assert.throws(()=>model.encode({},defs,{egress_mode:'automatico'},{}));
});
