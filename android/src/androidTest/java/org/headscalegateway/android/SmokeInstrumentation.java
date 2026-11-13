package org.headscalegateway.android;

import android.app.Instrumentation;
import android.content.*;
import android.os.*;
import org.json.*;
import java.io.*;
import java.net.*;
import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;
import java.util.concurrent.atomic.AtomicBoolean;

/** Solo se incorpora a la APK de pruebas; no existe en la aplicación distribuida. */
public final class SmokeInstrumentation extends Instrumentation {
    private Bundle arguments;
    String cookie="", csrf="";
    Context context;
    MainActivity activity;
    String browser(String code) throws Exception {
        java.util.concurrent.CountDownLatch complete=new java.util.concurrent.CountDownLatch(1);
        java.util.concurrent.atomic.AtomicReference<String> value=new java.util.concurrent.atomic.AtomicReference<>("");
        runOnMainSync(()->((android.webkit.WebView)activity.findViewById(R.id.panel_webview)).evaluateJavascript(code,result->{value.set(result);complete.countDown();}));
        require(complete.await(5,java.util.concurrent.TimeUnit.SECONDS),"El WebView no responde");return value.get();
    }
    private void capture(String name) throws Exception {
        if(!arguments.getString("capture","false").equals("true"))return;
        Thread.sleep(400);
        try(ParcelFileDescriptor result=getUiAutomation().executeShellCommand("screencap -p /data/local/tmp/hgw-ui-"+name+".png");
            InputStream stream=new ParcelFileDescriptor.AutoCloseInputStream(result)) {
            byte[] buffer=new byte[4096];while(stream.read(buffer)!=-1){}
        }
    }
    private void mobileInterface() throws Exception {
        if(arguments.getString("fullscreen","false").equals("true")) {
            try(ParcelFileDescriptor result=getUiAutomation().executeShellCommand("am start --windowingMode 1 -f 0x20000000 -n "+context.getPackageName()+"/org.headscalegateway.android.MainActivity");
                InputStream stream=new ParcelFileDescriptor.AutoCloseInputStream(result)) {byte[] bytes=new byte[1024];while(stream.read(bytes)!=-1){}}
            waitForIdleSync();
        }
        runOnMainSync(()->activity.findViewById(R.id.nav_panel).performClick());
        long deadline=System.currentTimeMillis()+20000;boolean ready=false;
        while(System.currentTimeMillis()<deadline) {
            if(browser("!!(document.getElementById('app') && !document.getElementById('app').hidden && window.gatewayAndroidSections && getComputedStyle(document.querySelector('.main-navigation')).display==='none')").equals("true")){ready=true;break;}
            Thread.sleep(300);
        }
        if(!ready) {
            capture("error");
            throw new AssertionError("El panel móvil no abrió su sesión: "+browser("JSON.stringify({url:location.href,app:document.getElementById('app')?.hidden,bridge:typeof window.gatewayAndroidSections,nav:document.querySelector('.main-navigation')?getComputedStyle(document.querySelector('.main-navigation')).display:null,body:document.body.innerText.slice(0,250)})"));
        }
        require(browser("getComputedStyle(document.querySelector('.main-navigation')).display").equals("\"none\""),"La navegación de escritorio ocupa la pantalla Android");
        browser("window.__idiomaInglesObservado=false;const idiomas=new MutationObserver(()=>{if(document.documentElement.lang==='en')window.__idiomaInglesObservado=true});idiomas.observe(document.documentElement,{attributes:true,attributeFilter:['lang']});document.getElementById('ui-language').value='en';document.getElementById('ui-language').dispatchEvent(new Event('change'))");
        long languageDeadline=System.currentTimeMillis()+5000;boolean spanish=false;
        while(System.currentTimeMillis()<languageDeadline){if(browser("window.__idiomaInglesObservado&&window.gatewayLanguage()==='es'").equals("true")){spanish=true;break;}Thread.sleep(100);}
        require(spanish,"El panel Android conserva un idioma anterior en vez de volver al español");
        require(browser("window.gatewayAndroidSections().some(s=>s.id==='services')").equals("true"),"El menú móvil pierde las funciones del panel común");
        JSONArray sections=new JSONArray(new JSONTokener(browser("JSON.stringify(window.gatewayAndroidSections())")).nextValue().toString());
        for(int i=0;i<sections.length();i++) {
            String section=sections.getJSONObject(i).getString("id");browser("window.gatewayAndroidOpenSection("+JSONObject.quote(section)+")");Thread.sleep(100);
            require(browser("document.documentElement.scrollWidth<=innerWidth+2").equals("true"),"La sección desborda la pantalla móvil: "+section);
        }
        browser("window.gatewayAndroidOpenSection('services')");
        browser("document.querySelectorAll('.toast-close').forEach(b=>b.click());window.scrollTo(0,0)");
        require(browser("!document.querySelector('[data-page=services]').hidden").equals("true"),"La navegación móvil no abre Servicios");
        capture("panel");
        java.util.concurrent.atomic.AtomicInteger height=new java.util.concurrent.atomic.AtomicInteger();
        runOnMainSync(()-> {
            android.view.View web=activity.findViewById(R.id.panel_webview);
            height.set((int)(100*web.getHeight()/(float)activity.getWindow().getDecorView().getHeight()));
            activity.findViewById(R.id.nav_help).performClick();require(!web.isShown(),"Ayuda no cambia de pantalla");
            activity.findViewById(R.id.nav_home).performClick();require(activity.findViewById(R.id.gateway_toggle).isShown(),"No se puede volver a Inicio");
        });
        require(height.get()>60,"El panel tiene poco espacio vertical: "+height.get()+" %");
        capture("inicio");
        runOnMainSync(()->activity.findViewById(R.id.nav_help).performClick());capture("ayuda");
        runOnMainSync(()->activity.findViewById(R.id.nav_home).performClick());
    }
    @Override public void onCreate(Bundle args) { super.onCreate(args); arguments=args; start(); }
    private void require(boolean ok,String message) { if(!ok) throw new AssertionError(message); }
    private void verifyRoutes() throws Exception {
        java.util.List<VpnRoutes.Prefix> routes=new java.util.ArrayList<>();
        VpnRoutes.remove(new VpnRoutes.Prefix(new byte[4],0),new VpnRoutes.Prefix(InetAddress.getByName("192.168.240.0").getAddress(),24),routes);
        java.math.BigInteger total=java.math.BigInteger.ZERO;
        for(VpnRoutes.Prefix route:routes) total=total.add(java.math.BigInteger.ONE.shiftLeft(32-route.length));
        require(total.equals(java.math.BigInteger.ONE.shiftLeft(32).subtract(java.math.BigInteger.valueOf(256))),"Las exclusiones IPv4 pierden rutas");
        for(String ip:new String[]{"100.64.0.2","8.8.8.8","192.168.239.255","192.168.241.0"})
            require(routes.stream().anyMatch(r->r.contains(prefix(ip))),"Destino IPv4 fuera del túnel: "+ip);
        require(routes.stream().noneMatch(r->r.contains(prefix("192.168.240.112"))),"La LAN quedó dentro del túnel");
        routes.clear();VpnRoutes.remove(new VpnRoutes.Prefix(new byte[16],0),prefix("::1"),routes);
        require(routes.stream().noneMatch(r->r.contains(prefix("::1"))),"Loopback IPv6 dentro del túnel");
        require(routes.stream().anyMatch(r->r.contains(prefix("fd7a:115c:a1e0::2"))),"La tailnet IPv6 quedó excluida");
    }
    private VpnRoutes.Prefix prefix(String address) {
        try {byte[] bytes=InetAddress.getByName(address).getAddress();return new VpnRoutes.Prefix(bytes,bytes.length*8);}
        catch(Exception e) {throw new AssertionError(e);}
    }
    void phase(JSONObject value) throws Exception {
        RuntimeState.write(new File(context.getFilesDir(),"lab-state.json"),value.toString());
        Bundle report=new Bundle();report.putString("lab",value.toString());sendStatus(2,report);
    }
    JSONObject api(String method,String path,JSONObject body) throws Exception {
        HttpURLConnection c=(HttpURLConnection)new URL(RuntimeState.base(context)+path).openConnection();
        c.setRequestMethod(method);c.setConnectTimeout(3000);c.setReadTimeout(15000);c.setInstanceFollowRedirects(false);
        c.setRequestProperty("Origin",RuntimeState.base(context));c.setRequestProperty("Content-Type","application/json");
        if(!cookie.isEmpty()) c.setRequestProperty("Cookie",cookie);
        if(!csrf.isEmpty()) c.setRequestProperty("X-CSRF-Token",csrf);
        try {
            if(body!=null) { c.setDoOutput(true);try(OutputStream out=c.getOutputStream()) {out.write(body.toString().getBytes(StandardCharsets.UTF_8));} }
            int code=c.getResponseCode();String raw=RuntimeState.read(code<400?c.getInputStream():c.getErrorStream());
            require(code<400,"API "+path+" devolvió "+code+": "+raw);
            if(c.getHeaderField("Set-Cookie")!=null) cookie=c.getHeaderField("Set-Cookie").split(";")[0];
            return raw.trim().startsWith("[")?new JSONObject().put("items",new JSONArray(raw)):new JSONObject(raw);
        } finally {c.disconnect();}
    }
    void login() throws Exception {
        JSONObject session=api("POST","/api/v1/auth/login",new JSONObject().put("username","android").put("password",RuntimeState.secret(context,"admin_password")));
        csrf=session.getString("csrf");
    }
    JSONObject activeCA()throws Exception {
        String domain=api("GET","/api/v1/status",null).getString("domain");
        JSONArray roots=api("GET","/api/v1/pki",null).getJSONArray("local_ca");
        for(int i=0;i<roots.length();i++)if(roots.getJSONObject(i).getString("name").equals(domain))return roots.getJSONObject(i);
        throw new AssertionError("No se encuentra la CA del dominio activo");
    }
    @Override public void onStart() {
        Bundle result=new Bundle();
        try {
            context=getTargetContext();
            if(arguments.getString("functional","false").equals("true") && !context.getPackageName().endsWith(".lab"))throw new AssertionError("Las pruebas funcionales requieren la APK de laboratorio aislada");
            if(Build.VERSION.SDK_INT>=33)getUiAutomation().grantRuntimePermission(context.getPackageName(),android.Manifest.permission.POST_NOTIFICATIONS);
            verifyRoutes();
            // Solo el proceso instrumentado hereda la clave efímera del laboratorio.
            if(arguments.containsKey("ca")) {File ca=new File(RuntimeState.root(context),"headscale-lab.crt");RuntimeState.write(ca,arguments.getString("ca"));android.system.Os.setenv("SSL_CERT_FILE",ca.getPath(),true);}
            if(arguments.containsKey("authkey")) android.system.Os.setenv("TS_AUTHKEY",arguments.getString("authkey"),true);
            if(arguments.containsKey("diagnostic")) {
                File log=new File(RuntimeState.root(context),"engine.log");
                byte[] bytes=new byte[8192]; int count;
                try(RandomAccessFile in=new RandomAccessFile(log,"r")) {if(arguments.getString("diagnostic").equals("tail"))in.seek(Math.max(0,in.length()-bytes.length));count=in.read(bytes);}
                result.putString("stream",count>0?new String(bytes,0,count,StandardCharsets.UTF_8):"Registro vacío");finish(-1,result);return;
            }
            Intent open=new Intent(context,MainActivity.class).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK);
            activity=(MainActivity)startActivitySync(open);
            RuntimeState.prefs(context).edit().putBoolean("desired_running",true).commit();
            context.startForegroundService(new Intent(context,GatewayService.class));
            long deadline=System.currentTimeMillis()+60000; boolean ready=false;
            while(System.currentTimeMillis()<deadline) { try { if(!GatewayService.running || RuntimeState.prefs(context).getBoolean("starting",false)) {Thread.sleep(100);continue;}RuntimeState.control(context,"/state");api("GET","/api/v1/auth/methods",null);ready=true;break; } catch(Exception e) {Thread.sleep(500);} }
            if(!ready) {
                String details=RuntimeState.prefs(context).getString("last_error","");
                File log=new File(RuntimeState.root(context),"engine.log");
                if(log.isFile()) {String text=RuntimeState.read(new FileInputStream(log)); details+="\n"+text.substring(0,Math.min(5000,text.length()));}
                throw new AssertionError("El motor no abrió su adaptador privado: "+details);
            }
            login(); require(api("GET","/api/v1/auth/session",null).getString("role").equals("admin"),"No se obtuvo una sesión administradora normal");
            HttpURLConnection denied=(HttpURLConnection)new URL("http://127.0.0.1:"+RuntimeState.prefs(context).getInt("control_port",0)+"/state").openConnection();
            require(denied.getResponseCode()==403,"El adaptador aceptó una petición sin credencial");denied.disconnect();
            api("GET","/api/v1/status",null);
            mobileInterface();
            String headscale=arguments.getString("headscale","");
            if(!headscale.isEmpty()) {
                JSONObject settings=new JSONObject().put("headscale.url",headscale).put("headscale.api_key",arguments.getString("apikey","")).put("domain.base_domain","lab.example.test")
                    .put("certificates.mode","force-local").put("dns.provider","none").put("acme.dns_provider","none")
                    .put("admin.domain_mode","tailnet").put("server.listen_addr","127.0.0.1:18443");
                api("PUT","/api/v1/config",new JSONObject().put("changes",settings));
                api("POST","/api/v1/system/restart",new JSONObject());
                Thread.sleep(4000);
                cookie="";csrf="";login();
                api("POST","/api/v1/services",new JSONObject().put("name","prueba").put("backend","http://127.0.0.1:18081"));
                deadline=System.currentTimeMillis()+180000; boolean connected=false;
                while(System.currentTimeMillis()<deadline) {
                    try {
                        JSONObject state=RuntimeState.control(context,"/state");
                        JSONObject info=api("GET","/api/v1/network/info",null);
                        phase(new JSONObject().put("phase","register").put("login_url",info.optString("login_url","")).put("state",info.optString("gateway_vpn_state","")));
                        if(state.optBoolean("connected")) { connected=true;break; }
                    } catch(Exception ignored) {}
                    Thread.sleep(1000);
                }
                require(connected,"El nodo Android no conectó a Headscale");
                // Un servicio de la aplicación permite comprobar el proxy HTTPS desde otro nodo.
                ServerSocket backend=new ServerSocket(18081,4,InetAddress.getByName("127.0.0.1"));
                AtomicBoolean completed=new AtomicBoolean();
                Thread service=new Thread(()-> {
                    while(!backend.isClosed()) try(Socket client=backend.accept()) {
                        client.setSoTimeout(3000);BufferedReader in=new BufferedReader(new InputStreamReader(client.getInputStream()));
                        String first=in.readLine();if(first!=null && first.startsWith("GET /done "))completed.set(true);
                        String line;while((line=in.readLine())!=null&&!line.isEmpty()) {}
                        byte[] payload="Servicio Android del Gateway comprobado".getBytes(StandardCharsets.UTF_8);
                        client.getOutputStream().write(("HTTP/1.1 200 OK\r\nContent-Length: "+payload.length+"\r\nConnection: close\r\n\r\n").getBytes(StandardCharsets.US_ASCII));client.getOutputStream().write(payload);
                    } catch(Exception ignored) {}
                },"servicio-prueba"); service.setDaemon(true);service.start();
                if(arguments.getString("functional","false").equals("true"))new FunctionalChecks(this).run();
                phase(new JSONObject().put("phase","connected").put("state",RuntimeState.control(context,"/state")).put("service_host","prueba."+api("GET","/api/v1/status",null).getString("domain")).put("ca_pem",activeCA().getString("pem")));
                if(arguments.getString("vpn","false").equals("true")) {
                    require(android.net.VpnService.prepare(context)==null,"Falta consentir la VPN en el emulador");
                    context.startForegroundService(new Intent(context,GatewayVpnService.class));
                    Thread.sleep(3000);
                    require(RuntimeState.phoneVPN(context).optString("phase").equals("connected"),"El estado de la VPN del teléfono no corresponde al proceso del túnel");
                    phase(new JSONObject().put("phase","vpn").put("state",RuntimeState.control(context,"/state")));
                }
                phase(new JSONObject().put("phase","ready").put("state",RuntimeState.control(context,"/state")));
                deadline=System.currentTimeMillis()+180000;
                while(!completed.get()&&System.currentTimeMillis()<deadline) Thread.sleep(500);
                backend.close();
                require(completed.get(),"No se completó la comprobación desde el otro nodo");
                context.stopService(new Intent(context,GatewayVpnService.class));
            }
            phase(new JSONObject().put("phase","passed"));
            result.putString("stream","Pruebas Android correctas: arranque, sesión normal, navegación móvil, ancho de las secciones y adaptador privado.\n");
            finish(-1,result);
        } catch(Throwable e) {
            try {phase(new JSONObject().put("phase","failed").put("error",String.valueOf(e)));} catch(Exception ignored) {}
            result.putString("stream","Fallo de la prueba Android: "+e+"\n");finish(0,result);
        }
    }
}
