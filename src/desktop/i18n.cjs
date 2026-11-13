'use strict';
const fs=require('node:fs'),path=require('node:path');
const directory=path.join(__dirname,'locales');
const codes=fs.readdirSync(directory).filter(f=>/^[a-z][a-z0-9-]*\.json$/.test(f)).map(f=>f.slice(0,-5)).sort();
const catalogs=Object.fromEntries(codes.map(code=>[code,JSON.parse(fs.readFileSync(path.join(directory,code+'.json'),'utf8'))]));
let language='es';
function setLanguage(code){language=codes.includes(code)?code:'es';return language;}
function tr(message,params={}) {return (catalogs[language][message]||message).replace(/\{(\w+)\}/g,(m,k)=>Object.hasOwn(params,k)?String(params[k]):m);}
module.exports={tr,setLanguage,getLanguage:()=>language,codes,catalog:()=>catalogs[language]};
