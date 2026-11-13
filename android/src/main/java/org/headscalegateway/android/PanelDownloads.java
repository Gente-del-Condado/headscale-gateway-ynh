package org.headscalegateway.android;

import android.content.Intent;
import android.net.Uri;
import android.webkit.*;
import android.util.Base64;
import android.widget.Toast;
import org.json.JSONObject;
import java.io.*;

/** Transferencias autenticadas del panel hacia el selector de documentos Android. */
final class PanelDownloads {
    private final MainActivity activity;
    private final WebView panel;
    private File pending;
    private FileOutputStream output;
    private String nonce="", filename="", mime="application/octet-stream";
    private volatile String trustedOrigin="";
    private boolean pickerLaunched=false,recoveryShown=false;
    private long size,maxBytes;
    private File metadata() {return new File(RuntimeState.root(activity),"pending-download.json");}
    PanelDownloads(MainActivity activity,WebView panel) {
        this.activity=activity;this.panel=panel;
        try {
            JSONObject state=new JSONObject(RuntimeState.read(new FileInputStream(metadata())));
            File file=new File(state.getString("path"));
            if(file.getCanonicalFile().getParentFile().equals(activity.getCacheDir().getCanonicalFile())
                && file.getName().startsWith("exportacion-gateway-") && file.isFile()) {
                pending=file;filename=state.getString("name");mime=state.getString("mime");
            } else metadata().delete();
        }catch(Exception ignored){}
    }
    void trust(String url) {
        String base=RuntimeState.base(activity);trustedOrigin=url!=null && url.startsWith(base+"/admin/")?base:"";
    }
    @JavascriptInterface public synchronized String beginBlob(String origin,String name,String type) {
        if(!RuntimeState.base(activity).equals(origin) || !origin.equals(trustedOrigin))return "";
        if(pending!=null) {failure("Termina o cancela la exportación anterior.");return "";}
        try {
            // Reserva espacio para el temporal y otra copia si el destino
            // elegido utiliza el mismo almacenamiento. Sin un límite fijo.
            maxBytes=Math.max(0,new android.os.StatFs(activity.getCacheDir().getPath()).getAvailableBytes()-64L*1024*1024)/2;
            pending=File.createTempFile("exportacion-gateway-",".tmp",activity.getCacheDir());
            android.system.Os.chmod(pending.getPath(),0600);output=new FileOutputStream(pending);
            nonce=RuntimeState.hexSecret();size=0;
            filename=name.replaceAll("[^A-Za-z0-9._-]","_");
            if(filename.length()>180 || filename.isEmpty())filename="exportacion-gateway.hgw";
            mime=type!=null && type.matches("[A-Za-z0-9.+-]+/[A-Za-z0-9.+-]+")?type:"application/octet-stream";
            return nonce;
        }catch(Exception e){close();failure("No se ha podido preparar la exportación.");return "";}
    }
    void begin(String url,String disposition,String type) {
        String base=RuntimeState.base(activity);
        if(!base.equals(trustedOrigin) || panel.getUrl()==null || !panel.getUrl().startsWith(base+"/admin/"))return;
        if(!url.startsWith("blob:"+base+"/") && !url.startsWith(base+"/api/v1/") && !url.startsWith(base+"/pki/v1/"))return;
        String name=URLUtil.guessFileName(url,disposition,type);
        panel.evaluateJavascript("window.gatewayAndroidDownload&&window.gatewayAndroidDownload("+JSONObject.quote(url)+","+JSONObject.quote(name)+")",null);
    }
    @JavascriptInterface public synchronized void append(String id,String data) throws IOException {
        if(output==null || !nonce.equals(id) || data.length()>65536)throw new IOException("Descarga no autorizada");
        byte[] bytes=Base64.decode(data,Base64.DEFAULT);size+=bytes.length;
        if(size>maxBytes) {close();failure("No hay espacio libre suficiente para completar la exportación.");throw new IOException("Espacio insuficiente para la exportación");}
        output.write(bytes);
    }
    @JavascriptInterface public synchronized void finish(String id) throws IOException {
        if(output==null || !nonce.equals(id))return;
        output.getFD().sync();output.close();output=null;nonce="";
        try {RuntimeState.write(metadata(),new JSONObject().put("path",pending.getPath()).put("name",filename).put("mime",mime).toString());}
        catch(Exception e){close();failure("No se ha podido conservar la exportación.");return;}
        activity.runOnUiThread(this::launchPicker);
    }
    private synchronized void launchPicker() {
            try {pickerLaunched=true;activity.startActivityForResult(new Intent(Intent.ACTION_CREATE_DOCUMENT).addCategory(Intent.CATEGORY_OPENABLE).setType(mime).putExtra(Intent.EXTRA_TITLE,filename),53);}
            catch(Exception e){synchronized(this){close();}failure("No se encuentra un selector de documentos.");}
    }
    synchronized void offerPending() {
        if(pending==null || output!=null || pickerLaunched || recoveryShown)return;
        recoveryShown=true;
        new android.app.AlertDialog.Builder(activity).setTitle("Exportación pendiente")
            .setMessage("Hay un archivo preparado que todavía no has guardado: "+filename)
            .setPositiveButton("Guardar archivo",(d,w)->launchPicker())
            .setNegativeButton("Descartar",(d,w)->close()).setCancelable(false).show();
    }
    @JavascriptInterface public synchronized void failed(String id) {
        if(output!=null && nonce.equals(id)){close();failure("No se ha podido descargar el archivo.");}
    }
    @JavascriptInterface public void securitySettings(String origin) {
        if(origin.equals(trustedOrigin) && origin.equals(RuntimeState.base(activity)))activity.runOnUiThread(()->activity.openCertificateSettings());
    }
    synchronized void save(Uri destination) {
        try {
            if(destination!=null && pending!=null) {
                if(!"content".equals(destination.getScheme()))throw new IOException("Selecciona un proveedor de documentos de Android");
                try(InputStream in=new FileInputStream(pending);OutputStream out=activity.getContentResolver().openOutputStream(destination,"w")) {
                    if(out==null)throw new IOException("No se puede abrir el destino");
                    byte[] bytes=new byte[32768];int n;while((n=in.read(bytes))!=-1)out.write(bytes,0,n);
                }
                failure("Archivo guardado.");
            }
        }catch(Exception e){failure("No se ha podido guardar la exportación.");}finally{close();}
    }
    synchronized void detach() {trustedOrigin="";if(output!=null)close();}
    synchronized void close() {try{if(output!=null)output.close();}catch(Exception ignored){}output=null;nonce="";if(pending!=null)pending.delete();pending=null;metadata().delete();pickerLaunched=false;recoveryShown=false;}
    private void failure(String message) {activity.runOnUiThread(()->Toast.makeText(activity,message,Toast.LENGTH_LONG).show());}
}
