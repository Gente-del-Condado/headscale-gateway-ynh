'use strict';
const {tr,getLanguage}=require('./i18n.cjs');
function endpoint(value) {
 const u=new URL(value);
 if(u.username||u.password||u.search||u.hash||/%(?:2f|5c)/i.test(u.pathname))throw Error(tr('Usa la dirección base, sin credenciales, consulta ni fragmento'));
 if(u.protocol!=='https:' && !(u.protocol==='http:'&&['127.0.0.1','[::1]','localhost'].includes(u.hostname)))throw Error(tr('HTTP solo se permite en loopback; usa HTTPS para acceso remoto'));
 return u.origin+u.pathname.replace(/\/+$/,'');
}
function pending(nodes){return nodes.flatMap(n=>[...new Set(n.available_routes||[])].filter(r=>!(n.approved_routes||[]).includes(r)).map(route=>({key:`${n.id}:${route}`,name:n.given_name||n.name||String(n.id),route})));}
async function request(base,token,path){
 const res=await fetch(endpoint(base)+'/api/v1/'+path,{headers:{Authorization:'Bearer '+token,'Accept-Language':getLanguage()},redirect:'error',signal:AbortSignal.timeout(10000)});
 if(!res.ok)throw Error(tr('API HTTP {status}: comprueba token, permisos y disponibilidad',{status:res.status}));
 const limit=4*1024*1024;
 if(Number(res.headers.get('content-length'))>limit){await res.body?.cancel();throw Error(tr('Respuesta demasiado grande'));}
 // Limita bytes mientras llegan: no acumular una respuesta ilimitada antes de comprobarla.
 const reader=res.body?.getReader();if(!reader)throw Error(tr('Respuesta vacía de la API'));
 const chunks=[];let size=0;
 try {
  while(true){const {done,value}=await reader.read();if(done)break;size+=value.byteLength;if(size>limit){await reader.cancel();throw Error(tr('Respuesta demasiado grande'));}chunks.push(value);}
 } finally {reader.releaseLock();}
 const body=new Uint8Array(size);let offset=0;for(const chunk of chunks){body.set(chunk,offset);offset+=chunk.byteLength;}
 return JSON.parse(new TextDecoder().decode(body));
}
module.exports={endpoint,pending,request};
