'use strict';
// Comprueba el portal activo en solo lectura; no abre servicios ni cambia ajustes.
const {app,BrowserWindow}=require('electron');
const fs=require('node:fs'),path=require('node:path');
app.whenReady().then(async()=>{try{
 const win=new BrowserWindow({width:1200,height:900,show:false,webPreferences:{sandbox:true,contextIsolation:true,nodeIntegration:false}});
 await win.loadURL(process.env.HGW_PORTAL_URL||'https://gateway.example.invalid/');
 const result=await win.webContents.executeJavaScript(`(()=>{
 const cards=[...document.querySelectorAll('.card')],s=document.querySelector('#search');
 if(!s||!cards.length)throw Error('Missing search or services');
 s.value='__no_service_matches_this_test__';s.dispatchEvent(new Event('input'));
 if(cards.some(c=>getComputedStyle(c).display!=='none'))throw Error('Search did not hide cards');
 if(document.querySelector('#none').hidden)throw Error('Missing empty state');
 s.value='';s.dispatchEvent(new Event('input'));
 if(cards.some(c=>c.hidden))throw Error('Search did not restore cards');
 if(document.querySelector('form'))throw Error('Unexpected administration form');
 return {cards:cards.length,search:true,emptyState:true};
 })()`);
 fs.writeFileSync(path.resolve(__dirname,'../../../docs/capturas/portal-servicios.png'),(await win.webContents.capturePage()).toPNG());
 console.log('PASS live portal',JSON.stringify(result));win.destroy();app.exit(0);
 }catch(e){console.error(e);app.exit(1);}});
