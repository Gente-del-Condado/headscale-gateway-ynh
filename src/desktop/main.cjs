'use strict';
const {app,BrowserWindow,Tray,Menu,Notification,ipcMain,shell,nativeImage,safeStorage,screen}=require('electron');
const fs=require('node:fs'),path=require('node:path');
const {popupBounds}=require('./window-layout.cjs');
const {endpoint,pending,request}=require('./client.cjs');
const {tr,setLanguage,codes,catalog}=require('./i18n.cjs');
let win,tray,quitting=false,busy=false,generation=0,seen=new Set(),failed=false;
let config={url:'http://127.0.0.1:9091',notifications:false,remember:false,language:'es'},token='',state={connected:false,message:'Configura la conexión',nodes:[],requests:[]};
const file=()=>path.join(app.getPath('userData'),'connection.json');
let storageAvailable=false,storageWrites=Promise.resolve();
function cryptoReady(){return storageAvailable;}
function persist(){
 const c={...config},savedToken=token;
 const write=async()=>{
  if(c.remember&&savedToken){if(!cryptoReady())throw Error(tr('No hay almacén seguro disponible. Desmarca Recordar token.'));c.secret=(await safeStorage.encryptStringAsync(savedToken)).toString('base64');}
  const temporary=file()+'.next';fs.writeFileSync(temporary,JSON.stringify(c),{mode:0o600});fs.renameSync(temporary,file());
 };
 const result=storageWrites.then(write);storageWrites=result.catch(()=>{});return result;
}
const appID='es.gentedelcondado.gateway.tray';
const toastID='{9D05163D-960A-4EAE-81C0-59D62D54B21E}';
function prepareNotifications(){
 if(process.platform!=='win32')return;
 app.setToastActivatorCLSID(toastID);
 const dir=path.join(app.getPath('appData'),'Microsoft','Windows','Start Menu','Programs');
 fs.mkdirSync(dir,{recursive:true});
 if(!shell.writeShortcutLink(path.join(dir,'Headscale Gateway Tray.lnk'),'create',{target:process.execPath,cwd:path.dirname(process.execPath),description:'Headscale Gateway',appUserModelId:appID,toastActivatorClsid:toastID}))throw Error(tr('No se pudo registrar el acceso directo para notificaciones'));
}
let preferencesOpen=false;
function position(){const bounds=tray?.getBounds();const display=bounds&&bounds.width>0?screen.getDisplayMatching(bounds):screen.getDisplayNearestPoint(screen.getCursorScreenPoint());win.setBounds(popupBounds(display.workArea,bounds,preferencesOpen));}
function show(){position();win.show();win.focus();}

function notify(body){body=tr(body);if(config.notifications&&Notification.isSupported()){const n=new Notification({title:'Headscale Gateway',body});n.on('click',show);n.show();}}
function publish(){if(win&&!win.isDestroyed())win.webContents.send('state',snapshot());if(tray){tray.setToolTip('Gateway · '+tr(state.connected?'conectado':'sin conexión'));tray.setContextMenu(Menu.buildFromTemplate([{label:tr('Abrir mini panel'),click:show},{label:tr('Abrir administración web'),click:()=>shell.openExternal(config.url+'/admin/')},{label:tr('Solicitudes pendientes: {count}',{count:state.requests.length}),click:show},{label:tr('Actualizar'),click:poll},{type:'separator'},{label:tr('Salir'),click:()=>app.quit()}]));}}
function snapshot(){return {...state,config:{...config},hasToken:!!token,secureStorage:cryptoReady(),languages:codes,messages:catalog()};}
async function poll(){if(busy||!token)return;busy=true;const rev=generation;try{const [info,data]=await Promise.all([request(config.url,token,'network/info'),request(config.url,token,'network/nodes')]);if(rev!==generation)return;const nodes=Array.isArray(data.nodes)?data.nodes:[],requests=pending(nodes),fresh=requests.filter(r=>!seen.has(r.key));if(fresh.length)notify(tr('{count} rutas pendientes de aprobación. Revisa el panel.',{count:fresh.length}));if(failed)notify('Se ha recuperado la conexión con el Gateway.');seen=new Set(requests.map(r=>r.key));failed=false;state={connected:true,message:info.gateway_vpn_state||'API disponible',nodes,requests,updated:new Date().toISOString()};}catch(e){if(rev!==generation)return;if(!failed)notify('No se puede consultar el Gateway. Revisa la conexión.');failed=true;state={...state,connected:false,message:e.message};}finally{busy=false;publish();}}
if(!app.requestSingleInstanceLock()){app.quit();}else{
 app.on('second-instance',()=>win&&show());app.on('before-quit',event=>{if(!quitting){event.preventDefault();quitting=true;storageWrites.finally(()=>app.quit());}});app.on('window-all-closed',()=>{});
 app.whenReady().then(async()=>{
  app.setAppUserModelId(appID);
  storageAvailable=await safeStorage.isAsyncEncryptionAvailable().catch(()=>false);
  if(process.platform==='linux'&&safeStorage.getSelectedStorageBackend()==='basic_text')storageAvailable=false;
  try{const c=JSON.parse(fs.readFileSync(file(),'utf8'));config={url:endpoint(c.url),notifications:!!c.notifications,remember:!!c.remember,language:setLanguage(c.language)};if(c.secret&&cryptoReady())token=(await safeStorage.decryptStringAsync(Buffer.from(c.secret,'base64'))).result;}catch(e){if(e.code!=='ENOENT')state.message='No se pudo recuperar la conexión guardada: '+e.message;}
  if(config.notifications){try{prepareNotifications();}catch{config.notifications=false;}}
  win=new BrowserWindow({width:340,height:360,show:false,title:'Gateway',frame:false,resizable:false,maximizable:false,minimizable:false,skipTaskbar:true,alwaysOnTop:true,hasShadow:true,autoHideMenuBar:true,webPreferences:{preload:path.join(__dirname,'preload.cjs'),contextIsolation:true,nodeIntegration:false,sandbox:true}});
  win.webContents.setWindowOpenHandler(()=>({action:'deny'}));win.webContents.on('will-navigate',e=>e.preventDefault());
  win.on('close',e=>{if(!quitting){e.preventDefault();win.hide();}});
  const icon=nativeImage.createFromPath(path.join(__dirname,'icon.png'));tray=new Tray(icon);tray.on('click',()=>win.isVisible()?win.hide():show());
  win.on('blur',()=>{if(!preferencesOpen)win.hide();});
  ipcMain.handle('hide',()=>win.hide());
  ipcMain.handle('view',(_e,preferences)=>{preferencesOpen=preferences===true;position();});
  ipcMain.handle('snapshot',()=>snapshot());
  ipcMain.handle('language',async(_e,code)=>{config.language=setLanguage(code);await persist();publish();return snapshot();});
  ipcMain.handle('configure',async(_e,c)=>{const url=endpoint(String(c.url));if(c.token&&(!/^[\x21-\x7e]{1,4096}$/.test(c.token)))throw Error(tr('Token inválido'));const nextToken=c.token||((url===config.url)?token:'');if(!nextToken)throw Error(tr('Introduce un token del Gateway'));if(c.remember&&!cryptoReady())throw Error(tr('Almacén seguro no disponible: desmarca Recordar token'));if(c.notifications)prepareNotifications();config={url,notifications:!!c.notifications,remember:!!c.remember,language:setLanguage(c.language||config.language)};token=nextToken;generation++;seen.clear();state={connected:false,message:'Conectando…',nodes:[],requests:[]};await persist();await poll();return snapshot();});
  ipcMain.handle('test-notification',()=>{if(!config.notifications)throw Error(tr('Activa y guarda las notificaciones primero'));if(!Notification.isSupported())throw Error(tr('El sistema no admite notificaciones'));notify('Aviso de prueba. Las solicitudes reales aparecerán aquí.');});
  ipcMain.handle('refresh',async()=>{await poll();return snapshot();});
  ipcMain.handle('open-admin',()=>shell.openExternal(config.url+'/admin/'));
  ipcMain.handle('disconnect',async()=>{generation++;token='';seen.clear();config.remember=false;await persist();state={connected:false,message:'Sesión desconectada',nodes:[],requests:[]};publish();return snapshot();});
  win.loadFile(path.join(__dirname,'index.html'));win.once('ready-to-show',show);publish();poll();setInterval(poll,30000);
 });
}
