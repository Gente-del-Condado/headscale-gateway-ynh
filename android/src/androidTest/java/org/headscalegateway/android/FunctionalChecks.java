package org.headscalegateway.android;

import android.app.*;
import android.content.*;
import android.net.Uri;
import android.os.*;
import android.view.accessibility.AccessibilityNodeInfo;
import org.json.*;
import java.io.*;
import java.net.*;
import java.nio.charset.StandardCharsets;
import java.security.*;
import java.security.cert.*;
import java.util.*;
import java.util.concurrent.atomic.AtomicReference;

/** Pruebas con escrituras y restauración: exclusivamente org.*.lab. */
final class FunctionalChecks {
    private final SmokeInstrumentation s;
    private final String password="contraseña-laboratorio-2026";
    private final AtomicReference<Uri> selected=new AtomicReference<>(),saved=new AtomicReference<>();
    private Instrumentation.ActivityMonitor monitor;
    FunctionalChecks(SmokeInstrumentation s){this.s=s;}
    private void require(boolean ok,String message){if(!ok)throw new AssertionError(message);}
    private JSONObject api(String method,String path,JSONObject body)throws Exception{return s.api(method,"/api/v1/"+path,body);}
    private void mark(String detail)throws Exception{s.phase(new JSONObject().put("phase","functional").put("detail",detail));}
    private String hash(byte[] bytes)throws Exception {StringBuilder result=new StringBuilder();for(byte b:MessageDigest.getInstance("SHA-256").digest(bytes))result.append(String.format(Locale.ROOT,"%02x",b&255));return result.toString();}
    private byte[] bytes(Uri uri)throws Exception {try(InputStream in=s.context.getContentResolver().openInputStream(uri);ByteArrayOutputStream out=new ByteArrayOutputStream()){byte[] b=new byte[32768];int n;while((n=in.read(b))!=-1)out.write(b,0,n);return out.toByteArray();}}
    private byte[] download(String path)throws Exception {
        HttpURLConnection c=(HttpURLConnection)new URL(RuntimeState.base(s.context)+"/api/v1/"+path).openConnection();c.setRequestProperty("Cookie",s.cookie);c.setConnectTimeout(5000);c.setReadTimeout(20000);
        try {require(c.getResponseCode()==200,"Falló la descarga autenticada: "+path);try(InputStream in=c.getInputStream();ByteArrayOutputStream out=new ByteArrayOutputStream()){byte[] b=new byte[32768];int n;while((n=in.read(b))!=-1)out.write(b,0,n);return out.toByteArray();}}finally{c.disconnect();}
    }
    private void reject(String method,String path,JSONObject body,int... status)throws Exception {
        try {api(method,path,body);throw new AssertionError("Se aceptó una operación inválida: "+path);}
        catch(AssertionError expected){int code=status.length==0?400:status[0];require(expected.getMessage().contains("devolvió "+code),expected.getMessage());}
    }
    private void waitJS(String expression,String error)throws Exception {
        long deadline=System.currentTimeMillis()+25000;
        while(System.currentTimeMillis()<deadline){if(s.browser(expression).equals("true"))return;Thread.sleep(200);}
        throw new AssertionError(error);
    }
    private void accept(String label)throws Exception {
        long deadline=System.currentTimeMillis()+10000;
        while(System.currentTimeMillis()<deadline){
            AccessibilityNodeInfo root=s.getUiAutomation().getRootInActiveWindow();
            if(root!=null)for(AccessibilityNodeInfo item:root.findAccessibilityNodeInfosByText(label))if(item.isClickable()&&item.getText()!=null&&label.equalsIgnoreCase(item.getText().toString().trim())){item.performAction(AccessibilityNodeInfo.ACTION_CLICK);return;}
            Thread.sleep(100);
        }throw new AssertionError("No apareció el diálogo: "+label);
    }
    private void pickers(){
        monitor=new Instrumentation.ActivityMonitor(){@Override public Instrumentation.ActivityResult onStartActivity(Intent intent){
            if(Intent.ACTION_CREATE_DOCUMENT.equals(intent.getAction())){
                String filename=intent.getStringExtra(Intent.EXTRA_TITLE);Uri uri=LabDocumentsProvider.uri(filename);saved.set(uri);
                return new Instrumentation.ActivityResult(Activity.RESULT_OK,new Intent().setData(uri).addFlags(Intent.FLAG_GRANT_WRITE_URI_PERMISSION));
            }
            Intent nested=intent.getParcelableExtra(Intent.EXTRA_INTENT);String action=nested==null?intent.getAction():nested.getAction();
            if(Intent.ACTION_OPEN_DOCUMENT.equals(action)&&selected.get()!=null)return new Instrumentation.ActivityResult(Activity.RESULT_OK,new Intent().setData(selected.get()).addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION));
            return null;
        }};s.addMonitor(monitor);
    }
    private void tapFileInput()throws Exception {
        s.browser("document.querySelectorAll('.toast-close').forEach(b=>b.click());document.getElementById('backup-import-file').scrollIntoView({block:'center',behavior:'instant'})");Thread.sleep(150);
        JSONObject rect=new JSONObject(new JSONTokener(s.browser("JSON.stringify((()=>{const r=document.getElementById('backup-import-file').getBoundingClientRect();return {x:r.x+r.width/2,y:r.y+r.height/2,width:innerWidth}})())")).nextValue().toString());
        int[] origin=new int[2];java.util.concurrent.atomic.AtomicInteger width=new java.util.concurrent.atomic.AtomicInteger();
        s.runOnMainSync(()->{android.view.View web=s.activity.findViewById(R.id.panel_webview);web.getLocationOnScreen(origin);width.set(web.getWidth());});
        float scale=width.get()/(float)rect.getDouble("width"),x=origin[0]+(float)rect.getDouble("x")*scale,y=origin[1]+(float)rect.getDouble("y")*scale;
        long now=SystemClock.uptimeMillis();android.view.MotionEvent down=android.view.MotionEvent.obtain(now,now,0,x,y,0),up=android.view.MotionEvent.obtain(now,now+80,1,x,y,0);
        s.getUiAutomation().injectInputEvent(down,true);s.getUiAutomation().injectInputEvent(up,true);down.recycle();up.recycle();
    }
    private void browserSession()throws Exception {
        long deadline=System.currentTimeMillis()+25000;
        while(System.currentTimeMillis()<deadline){
            s.browser("window.__sesionLaboratorio=false;fetch('/api/v1/auth/session',{credentials:'same-origin'}).then(r=>r.json()).then(v=>window.__sesionLaboratorio=!!v.authenticated&&typeof csrf!=='undefined'&&v.csrf===csrf).catch(()=>{})");Thread.sleep(200);
            if(s.browser("window.__sesionLaboratorio===true&&!!window.gatewayAndroidDownload").equals("true"))return;
        }throw new AssertionError("El panel no recupera su sesión después del reinicio");
    }
    private Uri exported(String script,String filename,byte[] expected)throws Exception {
        s.context.getContentResolver().delete(LabDocumentsProvider.uri(filename),null,null);
        saved.set(null);s.browser(script);
        long deadline=System.currentTimeMillis()+25000;
        while(System.currentTimeMillis()<deadline){
            Uri uri=saved.get();if(uri!=null)try{byte[] actual=bytes(uri);if(Arrays.equals(actual,expected)){require(filename.equals(uri.getLastPathSegment()),"Se perdió el nombre del archivo exportado");return uri;}}catch(FileNotFoundException ignored){}
            Thread.sleep(200);
        }throw new AssertionError("La descarga no guardó sus bytes mediante el selector Android: "+filename+"; destino="+saved.get()+"; "+s.browser("document.getElementById('notice')?.textContent"));
    }
    private void restart(String before)throws Exception {
        long deadline=System.currentTimeMillis()+60000;
        while(System.currentTimeMillis()<deadline){
            Thread.sleep(400);
            try {JSONObject state=RuntimeState.control(s.context,"/state");if(state.optString("boot_id").equals(before))continue;s.cookie="";s.csrf="";s.login();if(state.optBoolean("connected")){browserSession();return;}}catch(Exception|AssertionError ignored){}
        }throw new AssertionError("El Gateway no volvió a conectar después de restaurar");
    }
    void run()throws Exception {
        require(s.context.getPackageName().endsWith(".lab"),"Prohibido modificar una instalación habitual");pickers();
        try {
            s.runOnMainSync(()->s.activity.findViewById(R.id.nav_panel).performClick());
            waitJS("!!(document.getElementById('app')&&!document.getElementById('app').hidden&&window.gatewayAndroidDownload)","El panel no está preparado para las operaciones funcionales");
            mark("configuración y CRUD de servicios");
            JSONObject initial=api("GET","config",null);String headscale=initial.getString("headscale.url");
            String originalIPs=RuntimeState.control(s.context,"/state").getJSONArray("ips").toString();
            require(headscale.startsWith("https://"),"La conexión de laboratorio debe validar TLS de Headscale");
            require(initial.getString("headscale.api_key").isEmpty(),"Se expone la clave de Headscale al panel");
            api("GET","network/nodes",null);
            reject("PUT","config",new JSONObject().put("changes",new JSONObject().put("headscale.url","file:///etc/passwd")));
            require(api("GET","config",null).getString("headscale.url").equals(headscale),"La validación fallida altera la configuración de Headscale");
            JSONObject service=new JSONObject().put("name","ciclo").put("backend","http://127.0.0.1:18081");api("POST","services",service);
            service.put("description","Servicio editado en Android");api("PUT","services/ciclo",service);
            require(api("GET","services",null).toString().contains("Servicio editado en Android"),"La edición de servicios no persiste");api("DELETE","services/ciclo",new JSONObject());
            require(!api("GET","services",null).toString().contains("\"ciclo\""),"El servicio eliminado sigue publicado");
            mark("certificados y diálogos interactivos");
            JSONObject pki=api("GET","pki",null);require(pki.getString("platform").equals("android"),"Android se identifica como Linux para los certificados");
            require(pki.getJSONObject("helper").getString("mode").equals("manual-settings"),"Android ofrece un helper de escritorio");
            JSONObject ca=s.activeCA();String fingerprint=ca.getString("fingerprint");
            byte[] certificate=download("pki/ca/"+fingerprint+"/certificate");
            X509Certificate root=(X509Certificate)CertificateFactory.getInstance("X.509").generateCertificate(new ByteArrayInputStream(certificate));
            require(root.getBasicConstraints()>=0&&hash(root.getEncoded()).equals(fingerprint),"CA pública o huella incorrecta");root.checkValidity();root.verify(root.getPublicKey());
            require(!new String(certificate,StandardCharsets.UTF_8).contains("PRIVATE KEY"),"La descarga expone una clave privada");
            s.browser("window.__confirmacion=null;setTimeout(()=>window.__confirmacion=confirm('Comprobación funcional del laboratorio'),0)");accept("Cancelar");waitJS("window.__confirmacion===false","Cancelar no cancela la operación");
            s.browser("window.__confirmacion=null;setTimeout(()=>window.__confirmacion=confirm('Comprobación funcional del laboratorio'),0)");accept("Continuar");waitJS("window.__confirmacion===true","No se confirma la operación solicitada");
            mark("descargas, certificados y exportación portable");
            exported("(()=>{const url=URL.createObjectURL(new Blob(['Certificado de prueba'],{type:'application/x-pem-file'}));const a=document.createElement('a');a.href=url;a.download='certificado-prueba.crt';a.click();URL.revokeObjectURL(url)})()","certificado-prueba.crt","Certificado de prueba".getBytes(StandardCharsets.UTF_8));
            exported("window.gatewayAndroidDownload(location.origin+'/api/v1/pki/ca/"+fingerprint+"/certificate','ca-publica.crt')","ca-publica.crt",certificate);
            JSONObject profile=api("POST","config/transfer/export",new JSONObject().put("password",password).put("secrets",true));
            s.phase(new JSONObject().put("phase","profile-export").put("data",profile.getString("data")));
            JSONObject imported=api("POST","config/transfer/import",new JSONObject().put("password",password).put("data",profile.getString("data")));
            require(imported.getBoolean("preserved_local_access"),"El perfil portable cambia el acceso local");
            // Aplica las mismas opciones importadas antes de las operaciones de servicios.
            String old=RuntimeState.control(s.context,"/state").getString("boot_id");api("POST","system/restart",new JSONObject());restart(old);
            waitJS("!!(document.getElementById('app')&&!document.getElementById('app').hidden&&window.gatewayAndroidDownload)","No se recuperó la sesión WebView");
            mark("creación y descarga de la copia completa cifrada");
            if(!api("GET","backups",null).getBoolean("encryption_ready"))api("POST","backups/encryption",new JSONObject().put("password",password));
            JSONObject schedule=new JSONObject().put("interval_hours",24).put("keep",7).put("components",new JSONObject().put("sqlite",true));
            api("PUT","backups/schedule",schedule);
            require(api("GET","backups",null).getJSONObject("settings").getInt("interval_hours")==24,"La programación de copias no se guarda");
            schedule.put("interval_hours",0);api("PUT","backups/schedule",schedule);
            JSONObject partial=api("POST","backups",new JSONObject().put("password",password).put("components",new JSONObject().put("sqlite",true)));
            byte[] partialArchive=download("backups/"+partial.getString("id")+"/download");
            require(hash(partialArchive).equals(partial.getString("sha256")),"La copia SQLite parcial no conserva su integridad");
            s.phase(new JSONObject().put("phase","selected-export").put("data",android.util.Base64.encodeToString(partialArchive,android.util.Base64.NO_WRAP)));
            JSONObject parts=new JSONObject().put("full",true);
            String serviceSnapshot=api("GET","services",null).toString();
            reject("POST","backups",new JSONObject().put("password","contraseña incorrecta").put("components",parts));
            JSONObject backup=api("POST","backups",new JSONObject().put("password",password).put("components",parts));String id=backup.getString("id");
            byte[] archive=download("backups/"+id+"/download");require(hash(archive).equals(backup.getString("sha256")),"La copia descargada no coincide con SHA-256");
            require(archive.length<384*1024,"La copia de laboratorio supera el límite del canal de pruebas");
            s.phase(new JSONObject().put("phase","archive-export").put("data",android.util.Base64.encodeToString(archive,android.util.Base64.NO_WRAP)));
            Uri archiveFile=exported("window.gatewayAndroidDownload(location.origin+'/api/v1/backups/"+id+"/download',"+JSONObject.quote(id+".hgw")+")",id+".hgw",archive);
            // Permite comprobar el formato Android con el motor Linux sin publicar datos reales.
            try(OutputStream out=new FileOutputStream(new File(s.context.getFilesDir(),"laboratorio-exportado.hgw"))){out.write(archive);}
            mark("importación mediante el selector Android");
            s.browser("window.gatewayAndroidOpenSection('backups')");selected.set(archiveFile);
            tapFileInput();
            waitJS("document.getElementById('backup-import-file').files.length===1","El selector Android no entrega el archivo al panel");
            Set<String> previous=new HashSet<>();JSONArray beforeImport=api("GET","backups",null).getJSONArray("items");for(int i=0;i<beforeImport.length();i++)previous.add(beforeImport.getJSONObject(i).getString("id"));
            s.browser("document.getElementById('backup-import-password').value="+JSONObject.quote(password)+";setTimeout(()=>document.getElementById('backup-import').click(),0)");accept("Continuar");
            long deadline=System.currentTimeMillis()+30000;JSONObject uploaded=null;
            while(System.currentTimeMillis()<deadline){JSONArray entries=api("GET","backups",null).getJSONArray("items");for(int i=0;i<entries.length();i++)if(!previous.contains(entries.getJSONObject(i).getString("id"))&&entries.getJSONObject(i).getString("sha256").equals(backup.getString("sha256")))uploaded=entries.getJSONObject(i);if(uploaded!=null)break;Thread.sleep(300);}
            require(uploaded!=null,"El panel Android no importó la copia cifrada seleccionada");
            mark("restauración real y comprobación de identidad y certificados");
            api("DELETE","services/prueba",new JSONObject());
            api("PUT","config",new JSONObject().put("changes",new JSONObject().put("domain.base_domain","cambiado.example.test")));
            String boot=RuntimeState.control(s.context,"/state").getString("boot_id");
            reject("POST","backups/"+uploaded.getString("id")+"/restore",new JSONObject().put("password","incorrecta").put("confirm","RESTAURAR"),409);
            api("POST","backups/"+uploaded.getString("id")+"/restore",new JSONObject().put("password",password).put("confirm","RESTAURAR"));restart(boot);
            require(api("GET","config",null).getString("domain.base_domain").equals(initial.getString("domain.base_domain")),"La restauración no recupera la configuración");
            require(api("GET","services",null).toString().contains("\"prueba\""),"La restauración no recupera los servicios");
            require(api("GET","services",null).toString().equals(serviceSnapshot),"La restauración cambia el catálogo de servicios");
            require(s.activeCA().getString("fingerprint").equals(fingerprint),"La restauración pierde la CA original");
            require(api("GET","backups/restore/status",null).getString("phase").equals("completed"),"La restauración no terminó correctamente");
            require(RuntimeState.control(s.context,"/state").getJSONArray("ips").toString().equals(originalIPs),"La restauración cambia la identidad VPN");
            mark("funciones locales comprobadas; falta verificar HTTPS desde el segundo nodo");
        }finally{s.removeMonitor(monitor);}
    }
}
