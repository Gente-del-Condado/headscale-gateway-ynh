'use strict';
// Real Electron integration; isolated profile and loopback API, no production data.
const assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path');
const root=path.resolve(__dirname,'../..');
if(!process.versions.electron){
 const http=require('node:http'),{spawn}=require('node:child_process');
 const profile=fs.mkdtempSync(path.join(require('node:os').tmpdir(),'hgw-linux-check-'));
 let outage=false,routes=true;
 const server=http.createServer((req,res)=>{
  if(req.url==='/control/down')outage=true;
  if(req.url==='/control/up')outage=false;
  if(req.url==='/control/clear')routes=false;
  if(req.url.startsWith('/control/'))return res.end('ok');
  if(req.headers.authorization!=='Bearer local-integration-fixture'){res.writeHead(401);return res.end();}
  if(outage){res.writeHead(503);return res.end();}
  res.setHeader('Content-Type','application/json');
  res.end(JSON.stringify(req.url.endsWith('/info')?{gateway_vpn_state:'Linux lab'}:{nodes:[{id:1,name:'Linux lab',available_routes:routes?['192.0.2.0/24']:[],approved_routes:[]}]}));
 });
 (async()=>{try{
  await new Promise(r=>server.listen(0,'127.0.0.1',r));
  for(const stage of ['save','restore','empty','basic'])await new Promise((resolve,reject)=>{
   const child=spawn(require(path.join(root,'desktop/node_modules/electron')), [...(stage==='basic'?['--password-store=basic']:[]),__filename],{stdio:'inherit',env:{...process.env,HGW_CHECK_PROFILE:profile,HGW_CHECK_STAGE:stage,HGW_CHECK_URL:`http://127.0.0.1:${server.address().port}`}});
   const timeout=setTimeout(()=>{child.kill();reject(Error('Electron timeout'));},45000);
   child.on('error',reject);child.on('exit',code=>{clearTimeout(timeout);code===0?resolve():reject(Error(`Stage ${stage}: exit ${code}`));});
  });
  console.log('PASS Linux GUI integration: all four processes completed');
 }finally{server.close();fs.rmSync(profile,{recursive:true,force:true});}})().catch(e=>{console.error(e);process.exitCode=1;});
}else{
 const {app,BrowserWindow,safeStorage,Notification}=require('electron');
 app.setPath('userData',process.env.HGW_CHECK_PROFILE);
 app.setAppPath(path.join(root,'desktop'));
 require(path.join(root,'desktop/main.cjs'));
 app.whenReady().then(async()=>{try{
  const win=BrowserWindow.getAllWindows()[0];
  if(win.webContents.isLoading())await new Promise(r=>win.webContents.once('did-finish-load',r));
  assert.ok(win.webContents.getURL().endsWith('/desktop/index.html'));
  assert.equal(await win.webContents.executeJavaScript("!!document.getElementById('count')"),true);
  const call=code=>win.webContents.executeJavaScript(code);
  const stage=process.env.HGW_CHECK_STAGE,url=process.env.HGW_CHECK_URL;
  const configfile=path.join(process.env.HGW_CHECK_PROFILE,'connection.json');
  if(stage==='save'){
   console.log('Storage backend:',safeStorage.getSelectedStorageBackend());
   const s=await call(`gateway.configure(${JSON.stringify({url,token:'local-integration-fixture',remember:true,notifications:false})})`);
   assert.equal(s.connected,true);assert.equal(s.requests.length,1);assert.equal(s.hasToken,true);
   const localized=await call("gateway.language('en')");assert.equal(localized.config.language,'en');
   await new Promise(r=>setTimeout(r,100));
   assert.equal(await call("document.documentElement.lang"),'en');
   assert.match(await call("document.getElementById('disconnect').textContent"),/Disconnect and forget token/);

   assert.equal(await call('typeof require'),'undefined');
   const disk=fs.readFileSync(configfile,'utf8');assert.ok(!disk.includes('local-integration-fixture'));
   assert.ok(JSON.parse(disk).secret);assert.equal(fs.statSync(configfile).mode&0o777,0o600);
   assert.equal(safeStorage.decryptString(Buffer.from(JSON.parse(disk).secret,'base64')),'local-integration-fixture');
   await assert.rejects(call('gateway.testNotification()'));
   await fetch(url+'/control/down');assert.equal((await call('gateway.refresh()')).connected,false);
   await fetch(url+'/control/up');assert.equal((await call('gateway.refresh()')).connected,true);
   await fetch(url+'/control/clear');assert.equal((await call('gateway.refresh()')).requests.length,0);
   await call('gateway.configure({url:'+JSON.stringify(url)+',token:"",remember:true,notifications:true})');
   if(Notification.isSupported()){await call('gateway.testNotification()');console.log('Native notification requested (visual delivery requires manual confirmation)');}
   await call('gateway.configure({url:'+JSON.stringify(url)+',token:"",remember:true,notifications:false})');
   win.close();assert.equal(win.isDestroyed(),false);assert.equal(win.isVisible(),false);
  }else if(stage==='restore'){
   let s;for(let i=0;i<100;i++){s=await call('gateway.snapshot()');if(s.connected)break;await new Promise(r=>setTimeout(r,50));}
   assert.equal(s.hasToken,true);assert.equal(s.connected,true);assert.equal(s.config.url,url);assert.equal(s.config.language,'en');
   const d=await call('gateway.disconnect()');assert.equal(d.hasToken,false);
   const saved=JSON.parse(fs.readFileSync(configfile,'utf8'));assert.equal(saved.remember,false);assert.equal(saved.secret,undefined);
  }else if(stage==='basic'){
   assert.equal(safeStorage.getSelectedStorageBackend(),'basic_text');
   await assert.rejects(call(`gateway.configure(${JSON.stringify({url,token:'local-integration-fixture',remember:true,notifications:false})})`));
   assert.equal(JSON.parse(fs.readFileSync(configfile,'utf8')).secret,undefined);
   assert.equal((await call(`gateway.configure(${JSON.stringify({url,token:'local-integration-fixture',remember:false,notifications:false})})`)).connected,true);
  }else{const s=await call('gateway.snapshot()');assert.equal(s.hasToken,false);assert.equal(s.connected,false);}
  console.log('PASS stage '+stage);app.exit(0);
 }catch(e){console.error(e);app.exit(1);}});
}
