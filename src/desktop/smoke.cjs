// Ejecutar con el Electron empaquetado en un escritorio interactivo Linux o Windows.
const {app,BrowserWindow}=require('electron');
const http=require('node:http');
const assert=require('node:assert/strict');
let server;
app.setPath('userData',require('node:fs').mkdtempSync(require('node:path').join(require('node:os').tmpdir(),'hgw-tray-test-')));
require('./main.cjs');
app.whenReady().then(async()=>{
 try{
 server=http.createServer((req,res)=>{if(req.headers.authorization!=='Bearer fixture'){res.writeHead(401);res.end();return;}res.setHeader('Content-Type','application/json');res.end(JSON.stringify(req.url.endsWith('/info')?{gateway_vpn_state:'Running'}:{nodes:[{id:1,name:'laboratorio',online:true,ip_addresses:['100.64.0.10'],available_routes:['10.20.0.0/16'],approved_routes:[]}]}));});
 await new Promise(r=>server.listen(0,'127.0.0.1',r));
 const win=BrowserWindow.getAllWindows()[0]||await new Promise(resolve=>app.once('browser-window-created',(_e,created)=>resolve(created)));
 if(win.webContents.isLoading())await new Promise(r=>win.webContents.once('did-finish-load',r));
 const result=await win.webContents.executeJavaScript(`(async()=>{const s=await gateway.configure({url:'http://127.0.0.1:${server.address().port}',token:'fixture',remember:false,notifications:false});return {connected:s.connected,requests:s.requests.length,nodes:s.nodes.length,nodeIntegration:typeof require};})()`);
 assert.deepEqual(result,{connected:true,requests:1,nodes:1,nodeIntegration:'undefined'});
 await new Promise(r=>setTimeout(r,150));
 assert.equal(await win.webContents.executeJavaScript("document.getElementById('count').textContent"),'1');
 await win.webContents.executeJavaScript('view(false)');
 assert.equal(win.getBounds().width,340);assert.ok(win.getBounds().height<=360);
 assert.equal(await win.webContents.executeJavaScript('document.documentElement.scrollWidth<=window.innerWidth'),true);
 await win.webContents.executeJavaScript('view(true)');assert.ok(win.getBounds().height<=560);
 assert.equal(await win.webContents.executeJavaScript("document.getElementById('dashboard').hidden"),true);
 await win.webContents.executeJavaScript('view(false)');

 if(process.env.HGW_SCREENSHOT)require('node:fs').writeFileSync(process.env.HGW_SCREENSHOT,(await win.webContents.capturePage()).toPNG());
 const snapshot=await win.webContents.executeJavaScript('gateway.snapshot()');
 if(snapshot.secureStorage){
  await win.webContents.executeJavaScript(`gateway.configure({url:'http://127.0.0.1:${server.address().port}',token:'fixture',remember:true,notifications:false})`);
  const saved=JSON.parse(require('node:fs').readFileSync(require('node:path').join(app.getPath('userData'),'connection.json'),'utf8'));
  assert.ok(saved.secret);assert.ok(!JSON.stringify(saved).includes('fixture'));
  assert.equal((await require('electron').safeStorage.decryptStringAsync(Buffer.from(saved.secret,'base64'))).result,'fixture');
  console.log('PASS Electron: asynchronous secure storage encrypt/decrypt and encrypted persistence');
 }
 const disconnected=await win.webContents.executeJavaScript('gateway.disconnect()');assert.equal(disconnected.hasToken,false);
 assert.equal(JSON.parse(require('node:fs').readFileSync(require('node:path').join(app.getPath('userData'),'connection.json'),'utf8')).secret,undefined);
 win.close();assert.equal(win.isDestroyed(),false);assert.equal(win.isVisible(),false);
 console.log('PASS Electron: renderer sandbox, API auth, inventory, pending routes, disconnect, close-to-tray');
 server.close();app.quit();
 }catch(e){console.error(e);if(server)server.close();app.exit(1);}
});
