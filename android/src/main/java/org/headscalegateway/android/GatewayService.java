package org.headscalegateway.android;

import android.app.*;
import android.content.*;
import android.net.*;
import android.os.*;
import java.io.*;
import java.util.concurrent.*;

/** El proceso Android mantiene el motor común y respeta siempre la parada manual. */
public final class GatewayService extends Service {
    public static final String STOP="org.headscalegateway.android.STOP";
    static volatile boolean running=false;
    private java.lang.Process engine;
    private volatile boolean stopping=false;
    private final ExecutorService worker=Executors.newSingleThreadExecutor();
    private ConnectivityManager.NetworkCallback networks;

    static Notification notification(Context c,String text) {
        NotificationManager manager=c.getSystemService(NotificationManager.class);
        if(Build.VERSION.SDK_INT>=26) manager.createNotificationChannel(new NotificationChannel("gateway",c.getString(R.string.channel),NotificationManager.IMPORTANCE_LOW));
        Intent open=new Intent(c,MainActivity.class);
        PendingIntent content=PendingIntent.getActivity(c,0,open,PendingIntent.FLAG_IMMUTABLE|PendingIntent.FLAG_UPDATE_CURRENT);
        PendingIntent stop=PendingIntent.getService(c,1,new Intent(c,GatewayService.class).setAction(STOP),PendingIntent.FLAG_IMMUTABLE|PendingIntent.FLAG_UPDATE_CURRENT);
        Notification.Builder builder=Build.VERSION.SDK_INT>=26?new Notification.Builder(c,"gateway"):new Notification.Builder(c);
        return builder.setContentTitle(c.getString(R.string.app_name)).setContentText(text).setSmallIcon(android.R.drawable.ic_lock_lock)
            .setContentIntent(content).setOngoing(true).addAction(new Notification.Action.Builder(null,c.getString(R.string.stop),stop).build()).build();
    }

    @Override public int onStartCommand(Intent intent,int flags,int startId) {
        if(intent!=null && STOP.equals(intent.getAction())) {
            RuntimeState.prefs(this).edit().putBoolean("desired_running",false).commit();
            stopService(new Intent(this,GatewayVpnService.class)); stopSelf(); return START_NOT_STICKY;
        }
        startForeground(10,notification(this,getString(R.string.notification)));
        if(!RuntimeState.prefs(this).getBoolean("desired_running",false)) { stopSelf(); return START_NOT_STICKY; }
        if(engine==null && !running) { RuntimeState.prefs(this).edit().putBoolean("starting",true).commit();running=true; worker.execute(this::launch); }
        return START_STICKY;
    }

    private void launch() {
        try {
            RuntimeState.stopPrevious(this);
            Thread.sleep(250);
            RuntimeState.prepare(this);
            File data=new File(getFilesDir(),"gateway"); data.mkdirs();
            File executable=new File(getApplicationInfo().nativeLibraryDir,"libheadscale_gateway.so");
            ProcessBuilder builder=new ProcessBuilder(executable.getPath(),"--db",new File(data,"gateway.sqlite").getPath());
            builder.directory(getFilesDir()); builder.redirectErrorStream(true);
            File launchLog=new File(RuntimeState.root(this),"engine.log");
            RuntimeState.write(launchLog,"");
            builder.environment().put("GATEWAY_BOOTSTRAP_FILE",new File(RuntimeState.root(this),"startup.json").getPath());
            builder.environment().put("GATEWAY_ANDROID_NETWORK_FILE",new File(RuntimeState.root(this),"network.json").getPath());
            builder.environment().put("GATEWAY_ANDROID_SECRET",RuntimeState.secret(this,"bridge_secret"));
            builder.environment().put("GATEWAY_ANDROID_CONTROL_PORT",Integer.toString(RuntimeState.prefs(this).getInt("control_port",0)));
            builder.environment().put("GATEWAY_ANDROID_SOCKS_PORT",Integer.toString(RuntimeState.prefs(this).getInt("socks_port",0)));
            builder.environment().put("GATEWAY_LANG","es");
            builder.environment().put("HOME",getFilesDir().getPath());
            builder.environment().put("XDG_CONFIG_HOME",new File(getFilesDir(),"config").getPath());
            builder.environment().put("TMPDIR",getCacheDir().getPath());
            synchronized(this) { if(stopping) return; engine=builder.start(); }
            java.lang.Process child=engine;
            Thread log=new Thread(()-> {
                try(InputStream in=child.getInputStream();FileOutputStream out=new FileOutputStream(launchLog,true)) {
                    byte[] buffer=new byte[8192];int n;while((n=in.read(buffer))!=-1)out.write(buffer,0,n);
                } catch(IOException e) {
                    if(!stopping)RuntimeState.prefs(this).edit().putString("last_error","No se puede escribir el registro privado del Gateway.").apply();
                }
            },"registro-gateway");log.setDaemon(true);log.start();
            RuntimeState.prefs(this).edit().putBoolean("starting",false).commit();
            ConnectivityManager manager=getSystemService(ConnectivityManager.class);
            networks=new ConnectivityManager.NetworkCallback() {
                private void changed() { new Thread(()-> { try { RuntimeState.network(GatewayService.this); RuntimeState.control(GatewayService.this,"/network"); } catch(Exception ignored) {} },"red-gateway").start(); }
                @Override public void onAvailable(Network n) { changed(); }
                @Override public void onLost(Network n) { changed(); }
                @Override public void onLinkPropertiesChanged(Network n,LinkProperties p) { changed(); }
            };
            manager.registerNetworkCallback(new NetworkRequest.Builder().addCapability(NetworkCapabilities.NET_CAPABILITY_NOT_VPN).build(),networks);
            engine.waitFor();
            log.join(1000);
            if(!stopping) RuntimeState.prefs(this).edit().putString("last_error","El motor del Gateway se ha detenido. Revisa su configuración.").apply();
        } catch(Exception e) {
            RuntimeState.prefs(this).edit().putString("last_error",getString(R.string.error,e.getMessage())).apply();
        } finally { RuntimeState.prefs(this).edit().putBoolean("starting",false).commit();running=false; stopService(new Intent(this,GatewayVpnService.class)); stopSelf(); }
    }
    @Override public void onDestroy() {
        synchronized(this) { stopping=true; if(engine!=null) engine.destroy(); }
        if(networks!=null) { try { getSystemService(ConnectivityManager.class).unregisterNetworkCallback(networks); } catch(Exception ignored) {} }
        worker.shutdown(); running=false; stopForeground(true); super.onDestroy();
    }
    @Override public IBinder onBind(Intent intent) { return null; }
}
