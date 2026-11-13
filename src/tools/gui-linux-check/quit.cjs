'use strict';
// Ejecuta la función real del menú de la aplicación con el entorno Electron real.
const {app,Menu,BrowserWindow}=require('electron');
const fs=require('node:fs'),path=require('node:path'),assert=require('node:assert/strict');
const root=path.resolve(__dirname,'../..');
const profile=fs.mkdtempSync(path.join(require('node:os').tmpdir(),'hgw-quit-check-'));
app.setPath('userData',profile);app.setAppPath(path.join(root,'desktop'));
let menu,invoked=false,before=false,will=false;
const build=Menu.buildFromTemplate.bind(Menu);
Menu.buildFromTemplate=template=>{menu=template;return build(template);};
app.on('before-quit',()=>{before=true;console.log('before-quit');});
app.on('will-quit',()=>{will=true;console.log('will-quit');});
app.on('quit',()=>{assert.ok(invoked&&before&&will);fs.rmSync(profile,{recursive:true,force:true});console.log('PASS actual Salir callback closes Electron');});
require(path.join(root,'desktop/main.cjs'));
app.whenReady().then(async()=>{
 const win=BrowserWindow.getAllWindows()[0];
 if(win.webContents.isLoading())await new Promise(r=>win.webContents.once('did-finish-load',r));
 win.close();assert.equal(win.isDestroyed(),false);
 const item=menu.find(item=>item.label==='Salir');assert.ok(item);
 invoked=true;item.click();
 setTimeout(()=>{console.error('FAIL quit timed out');app.exit(1);},5000).unref();
}).catch(e=>{console.error(e);app.exit(1);});
