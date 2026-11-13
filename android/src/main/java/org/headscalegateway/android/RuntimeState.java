package org.headscalegateway.android;

import android.content.Context;
import android.content.SharedPreferences;
import android.net.*;
import android.security.keystore.KeyGenParameterSpec;
import android.security.keystore.KeyProperties;
import android.system.Os;
import android.util.Base64;
import org.json.*;
import javax.crypto.*;
import javax.crypto.spec.GCMParameterSpec;
import java.io.*;
import java.net.*;
import java.nio.charset.StandardCharsets;
import java.security.*;
import java.util.*;

/** Contrato de arranque común y secretos privados protegidos por Android. */
final class RuntimeState {
    static SharedPreferences prefs(Context c) { return c.getSharedPreferences("gateway", Context.MODE_PRIVATE); }
    static File root(Context c) { return c.getNoBackupFilesDir(); }
    // SharedPreferences no sincroniza cachés entre el proceso principal y :vpn.
    static JSONObject phoneVPN(Context c) {
        try {
            JSONObject state=new JSONObject(read(new FileInputStream(new File(root(c),"phone-vpn.json"))));
            String phase=state.optString("phase");
            if(phase.equals("connected") || phase.equals("starting")) {
                int pid=state.getInt("pid");Os.kill(pid,0);
                String command=read(new FileInputStream("/proc/"+pid+"/cmdline")).split("\u0000")[0];
                if(!command.equals(c.getPackageName()+":vpn"))return new JSONObject();
            }
            return state;
        } catch(Exception ignored) {return new JSONObject();}
    }
    static synchronized void phoneVPN(Context c,String phase,String error) {
        try {
            write(new File(root(c),"phone-vpn.json"),new JSONObject().put("phase",phase)
                .put("pid",android.os.Process.myPid()).put("error",error).toString());
        } catch(Exception ignored) {}
    }
    static String base(Context c) { return "http://127.0.0.1:" + prefs(c).getInt("admin_port", 0); }
    static String hexSecret() { byte[] v = new byte[32]; new SecureRandom().nextBytes(v); StringBuilder b=new StringBuilder(); for(byte x:v) b.append(String.format(Locale.ROOT,"%02x",x & 255)); return b.toString(); }

    static synchronized String secret(Context c, String name) throws Exception {
        KeyStore store=KeyStore.getInstance("AndroidKeyStore"); store.load(null);
        String alias="headscale-gateway-private";
        if (!store.containsAlias(alias)) {
            KeyGenerator generator=KeyGenerator.getInstance(KeyProperties.KEY_ALGORITHM_AES,"AndroidKeyStore");
            generator.init(new KeyGenParameterSpec.Builder(alias,KeyProperties.PURPOSE_ENCRYPT|KeyProperties.PURPOSE_DECRYPT)
                .setBlockModes(KeyProperties.BLOCK_MODE_GCM).setEncryptionPaddings(KeyProperties.ENCRYPTION_PADDING_NONE).build());
            generator.generateKey();
        }
        SecretKey key=(SecretKey)store.getKey(alias,null);
        String encoded=prefs(c).getString(name,null);
        Cipher cipher=Cipher.getInstance("AES/GCM/NoPadding");
        if(encoded!=null) {
            byte[] bytes=Base64.decode(encoded,Base64.NO_WRAP);
            cipher.init(Cipher.DECRYPT_MODE,key,new GCMParameterSpec(128,Arrays.copyOfRange(bytes,0,12)));
            return new String(cipher.doFinal(Arrays.copyOfRange(bytes,12,bytes.length)),StandardCharsets.UTF_8);
        }
        String value=hexSecret(); cipher.init(Cipher.ENCRYPT_MODE,key);
        byte[] body=cipher.doFinal(value.getBytes(StandardCharsets.UTF_8));
        byte[] bytes=new byte[12+body.length]; System.arraycopy(cipher.getIV(),0,bytes,0,12); System.arraycopy(body,0,bytes,12,body.length);
        prefs(c).edit().putString(name,Base64.encodeToString(bytes,Base64.NO_WRAP)).commit();
        return value;
    }

    static void write(File file,String text) throws Exception {
        file.getParentFile().mkdirs();
        File temporary=new File(file.getPath()+".tmp");
        try(FileOutputStream out=new FileOutputStream(temporary)) { Os.chmod(temporary.getPath(),0600); out.write(text.getBytes(StandardCharsets.UTF_8)); out.getFD().sync(); }
        Os.rename(temporary.getPath(),file.getPath());
    }

    static void prepare(Context c) throws Exception {
        try(ServerSocket a=new ServerSocket(0,0,InetAddress.getByName("127.0.0.1"));
            ServerSocket b=new ServerSocket(0,0,InetAddress.getByName("127.0.0.1"));
            ServerSocket d=new ServerSocket(0,0,InetAddress.getByName("127.0.0.1"))) {
            prefs(c).edit().putInt("admin_port",a.getLocalPort()).putInt("control_port",b.getLocalPort()).putInt("socks_port",d.getLocalPort()).commit();
        }
        secret(c,"bridge_secret");
        File initial=new File(root(c),"initial-admin.json");
        write(initial,new JSONObject().put("username","android").put("password",secret(c,"admin_password")).toString());
        JSONObject options=new JSONObject().put("listen_addr","127.0.0.1:"+prefs(c).getInt("admin_port",0))
            .put("node_name","android-gateway").put("initial_admin_file",initial.getPath());
        write(new File(root(c),"startup.json"),options.toString());
        network(c);
    }

    static void stopPrevious(Context c) {
        // Solo procesos de nuestro UID y del ejecutable que usa esta SQLite.
        File[] entries=new File("/proc").listFiles();if(entries==null)return;
        String database=new File(c.getFilesDir(),"gateway/gateway.sqlite").getPath();
        for(File entry:entries) {
            try {
                int pid=Integer.parseInt(entry.getName());
                if(pid==android.os.Process.myPid() || Os.stat(entry.getPath()).st_uid!=android.os.Process.myUid()) continue;
                String args=read(new FileInputStream(new File(entry,"cmdline")));
                String[] values=args.split("\u0000");
                if(values.length>=3 && values[0].endsWith("/libheadscale_gateway.so") && values[1].equals("--db") && values[2].equals(database)) Os.kill(pid,15);
            } catch(Exception ignored) {}
        }
    }

    static synchronized void network(Context c) throws Exception {
        ConnectivityManager manager=(ConnectivityManager)c.getSystemService(Context.CONNECTIVITY_SERVICE);
        JSONArray interfaces=new JSONArray(); String primary="";
        for(Network n:manager.getAllNetworks()) {
            NetworkCapabilities caps=manager.getNetworkCapabilities(n); LinkProperties lp=manager.getLinkProperties(n);
            if(caps==null || lp==null || !caps.hasCapability(NetworkCapabilities.NET_CAPABILITY_NOT_VPN) || lp.getInterfaceName()==null) continue;
            JSONArray addresses=new JSONArray(); for(LinkAddress addr:lp.getLinkAddresses()) addresses.put(addr.toString());
            String name=lp.getInterfaceName(); NetworkInterface iface=NetworkInterface.getByName(name);
            int mtu=1500;
            try {mtu=android.os.Build.VERSION.SDK_INT>=29?lp.getMtu():iface!=null?iface.getMTU():1500;} catch(Exception ignored) {}
            if(mtu<=0)mtu=1500;
            interfaces.put(new JSONObject().put("name",name).put("index",iface==null?1:iface.getIndex())
                .put("mtu",mtu).put("addresses",addresses));
            if(caps.hasCapability(NetworkCapabilities.NET_CAPABILITY_INTERNET) && (primary.isEmpty() || n.equals(manager.getActiveNetwork()))) primary=name;
        }
        interfaces.put(new JSONObject().put("name","lo").put("index",1).put("mtu",65536).put("addresses",new JSONArray().put("127.0.0.1/8").put("::1/128")));
        write(new File(root(c),"network.json"),new JSONObject().put("default",primary).put("interfaces",interfaces).toString());
    }

    static JSONObject control(Context c,String path) throws Exception {
        URL url=new URL("http://127.0.0.1:"+prefs(c).getInt("control_port",0)+path);
        HttpURLConnection conn=(HttpURLConnection)url.openConnection();
        conn.setConnectTimeout(2000); conn.setReadTimeout(4000); conn.setInstanceFollowRedirects(false);
        conn.setRequestProperty("Authorization","Bearer "+secret(c,"bridge_secret"));
        if(path.equals("/network")) conn.setRequestMethod("POST");
        try {
            int status=conn.getResponseCode();
            if(status==204) return new JSONObject();
            if(status!=200) throw new IOException("El adaptador del Gateway no está disponible");
            return new JSONObject(read(conn.getInputStream()));
        } finally { conn.disconnect(); }
    }
    static String read(InputStream input) throws IOException {
        try(InputStream in=input; ByteArrayOutputStream out=new ByteArrayOutputStream()) {
            byte[] buffer=new byte[4096]; int n; while((n=in.read(buffer))!=-1) {
                if(out.size()+n>65536) throw new IOException("Respuesta demasiado grande"); out.write(buffer,0,n);
            }
            return out.toString("UTF-8");
        }
    }
}
