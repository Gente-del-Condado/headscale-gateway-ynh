'use strict';
const path=require('node:path');
const fs=require('node:fs');
const {execFileSync}=require('node:child_process');
(async()=>{
 const {packager}=await import('@electron/packager');
 const target=process.argv[2]||process.platform;
 if(!['all','win32','linux'].includes(target))throw Error('Platform must be win32, linux or all');
 const result=await packager({dir:__dirname,name:'Gateway-Tray',platform:target==='all'?['win32','linux']:target,arch:'x64',out:path.join(__dirname,'dist'),overwrite:true,asar:true,ignore:[/^[/\\]dist(?:[/\\]|$)/,/(?:\.test\.cjs|[/\\](?:smoke|pack)\.cjs)$/]});
 for(const folder of result){
  const platform=path.basename(folder).includes('-win32-')?'win32':'linux';
  if(!['win32','linux'].includes(target)&&target!=='all')continue;
  const destination=path.join(__dirname,'..','..','bin',...(platform==='linux'?['linux','Gateway-Tray']:['Gateway-Tray']));
  // La sustitución preparada aparte impide incluir un paquete incompleto en la entrega.
  const staged=destination+'.next';fs.rmSync(staged,{recursive:true,force:true});fs.cpSync(folder,staged,{recursive:true});
  // Ningún runtime sustituye la entrega anterior sin fuentes y avisos verificados.
  execFileSync(process.env.PYTHON||'python3',[path.join(__dirname,'..','..','tools','desktop_compliance.py'),'prepare',staged,'--platform',platform],{stdio:'inherit'});
  fs.rmSync(destination,{recursive:true,force:true});fs.renameSync(staged,destination);
 }
 console.log(result.join('\n'));
})().catch(e=>{console.error(e.message);process.exitCode=1;});
