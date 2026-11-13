package org.headscalegateway.android;

import android.Manifest;
import android.app.*;
import android.content.*;
import android.graphics.Color;
import android.graphics.Typeface;
import android.graphics.drawable.GradientDrawable;
import android.net.*;
import android.os.*;
import android.view.*;
import android.webkit.*;
import android.widget.*;
import org.json.*;
import java.io.File;
import java.net.URL;
import java.net.HttpURLConnection;
import java.nio.charset.StandardCharsets;
import java.util.concurrent.*;

/** Contenedor móvil del mismo panel y motor; no mantiene otra configuración. */
public final class MainActivity extends Activity {
    private static final int BG=0xff101827, SURFACE=0xff1c293b, TEXT=0xffedf4fc,
        MUTED=0xffacbdd1, ACCENT=0xff38bdf8, BORDER=0xff324258;
    private final Handler ui=new Handler(Looper.getMainLooper());
    private final ExecutorService worker=Executors.newSingleThreadExecutor();
    private WebView panel;
    private TextView gatewayTitle,gatewayDetail,nodeInfo,vpnDetail,headerCaption,errorText,placeholderText;
    private Button gatewayButton,vpnButton,sectionsButton;
    private final Button[] navigation=new Button[3];
    private View home,help,panelPlaceholder,dnsHelp;
    private FrameLayout panelPage;
    private ProgressBar loading;
    private PanelDownloads downloads;
    private ValueCallback<Uri[]> fileResult;
    private int screen=0,loadedPort=-1;
    private Runnable clearBackCallback;
    private String loadedBoot="",pendingSection="",mobileJS="";
    private boolean checking=false,visible=false,destroyed=false,engineReady=false,nodeConnected=false;
    private boolean phoneConnected=false,phoneStarting=false;

    private int dp(float value) {return Math.round(value*getResources().getDisplayMetrics().density);}
    private GradientDrawable shape(int color,int radius) {
        GradientDrawable d=new GradientDrawable();d.setColor(color);d.setCornerRadius(dp(radius));d.setStroke(dp(1),BORDER);return d;
    }
    private TextView text(String value,int size,int color) {
        TextView v=new TextView(this);v.setText(value);v.setTextSize(size);v.setTextColor(color);
        v.setLineSpacing(dp(3),1);v.setTextIsSelectable(false);return v;
    }
    private LinearLayout column() {LinearLayout v=new LinearLayout(this);v.setOrientation(LinearLayout.VERTICAL);return v;}
    private void space(LinearLayout parent,int height) {parent.addView(new View(this),new LinearLayout.LayoutParams(1,dp(height)));}
    private TextView heading(LinearLayout parent,String value) {
        TextView v=text(value,22,TEXT);v.setTypeface(null,Typeface.BOLD);parent.addView(v);return v;
    }
    private Button button(LinearLayout parent,String label,boolean primary,Runnable action) {
        Button b=new Button(this);b.setText(label);b.setAllCaps(false);b.setTextSize(15);
        b.setTextColor(primary?BG:TEXT);b.setBackground(shape(primary?ACCENT:SURFACE,12));
        b.setMinHeight(dp(50));b.setMinimumHeight(dp(50));b.setPadding(dp(14),dp(10),dp(14),dp(10));
        b.setOnClickListener(v->action.run());LinearLayout.LayoutParams lp=new LinearLayout.LayoutParams(-1,-2);lp.topMargin=dp(10);
        parent.addView(b,lp);return b;
    }
    private LinearLayout card(LinearLayout parent) {
        LinearLayout v=column();v.setPadding(dp(20),dp(20),dp(20),dp(20));v.setBackground(shape(SURFACE,18));
        LinearLayout.LayoutParams lp=new LinearLayout.LayoutParams(-1,-2);lp.bottomMargin=dp(16);parent.addView(v,lp);return v;
    }
    private ScrollView scroll(LinearLayout content) {
        ScrollView scroll=new ScrollView(this);scroll.setFillViewport(true);scroll.setClipToPadding(false);
        scroll.setPadding(dp(16),dp(20),dp(16),dp(20));
        FrameLayout center=new FrameLayout(this);center.addView(content,new FrameLayout.LayoutParams(-1,-2,Gravity.TOP|Gravity.CENTER_HORIZONTAL));
        center.addOnLayoutChangeListener((v,l,t,r,b,ol,ot,or,ob)-> {
            int width=Math.min(r-l,dp(760));FrameLayout.LayoutParams p=(FrameLayout.LayoutParams)content.getLayoutParams();
            if(width>0 && p.width!=width) {p.width=width;content.setLayoutParams(p);}
        });scroll.addView(center);return scroll;
    }
    private void post(Runnable action) {ui.post(()-> {if(!destroyed)action.run();});}

    @Override public void onCreate(Bundle saved) {
        super.onCreate(saved);
        if(Build.VERSION.SDK_INT>=30)getWindow().setDecorFitsSystemWindows(false);
        else getWindow().getDecorView().setSystemUiVisibility(View.SYSTEM_UI_FLAG_LAYOUT_STABLE|View.SYSTEM_UI_FLAG_LAYOUT_FULLSCREEN|View.SYSTEM_UI_FLAG_LAYOUT_HIDE_NAVIGATION);
        try {mobileJS=RuntimeState.read(getAssets().open("mobile-panel.js"));}
        catch(Exception e) {showError(e);}
        LinearLayout root=column();root.setBackgroundColor(BG);
        root.setOnApplyWindowInsetsListener((v,insets)-> {
            if(Build.VERSION.SDK_INT>=30) {
                android.graphics.Insets bars=insets.getInsets(WindowInsets.Type.systemBars()|WindowInsets.Type.displayCutout());
                int bottom=Math.max(bars.bottom,insets.getInsets(WindowInsets.Type.ime()).bottom);
                v.setPadding(bars.left,bars.top,bars.right,bottom);
            } else v.setPadding(insets.getSystemWindowInsetLeft(),insets.getSystemWindowInsetTop(),insets.getSystemWindowInsetRight(),insets.getSystemWindowInsetBottom());
            return insets;
        });
        LinearLayout toolbar=new LinearLayout(this);toolbar.setGravity(Gravity.CENTER_VERTICAL);toolbar.setPadding(dp(18),dp(4),dp(12),dp(4));
        LinearLayout brand=column();TextView title=text("◈  Headscale Gateway",18,TEXT);title.setTypeface(null,Typeface.BOLD);
        title.setSingleLine(true);title.setEllipsize(android.text.TextUtils.TruncateAt.END);brand.addView(title);
        headerCaption=text("Tu Gateway, también en Android",12,MUTED);headerCaption.setSingleLine(true);headerCaption.setEllipsize(android.text.TextUtils.TruncateAt.END);brand.addView(headerCaption);
        root.addOnLayoutChangeListener((v,l,t,r,b,ol,ot,or,ob)->headerCaption.setVisibility(b-t<dp(500)?View.GONE:View.VISIBLE));
        toolbar.addView(brand,new LinearLayout.LayoutParams(0,-2,1));
        sectionsButton=new Button(this);sectionsButton.setId(R.id.panel_sections);sectionsButton.setText("Secciones ▾");sectionsButton.setAllCaps(false);
        sectionsButton.setTextColor(ACCENT);sectionsButton.setTextSize(13);sectionsButton.setMinWidth(0);sectionsButton.setOnClickListener(v->showSections());
        toolbar.addView(sectionsButton,new LinearLayout.LayoutParams(-2,dp(48)));root.addView(toolbar);
        FrameLayout pages=new FrameLayout(this);home=buildHome();help=buildHelp();panelPage=buildPanel();
        pages.addView(home);pages.addView(panelPage);pages.addView(help);root.addView(pages,new LinearLayout.LayoutParams(-1,0,1));
        LinearLayout bar=new LinearLayout(this);bar.setPadding(dp(8),dp(4),dp(8),dp(4));bar.setBackgroundColor(0xff0b1220);
        String[] labels={"Inicio","Panel","Ayuda"};int[] ids={R.id.nav_home,R.id.nav_panel,R.id.nav_help};
        for(int i=0;i<3;i++) {
            final int page=i;Button tab=new Button(this);tab.setId(ids[i]);tab.setText(labels[i]);tab.setAllCaps(false);tab.setTextSize(14);
            tab.setOnClickListener(v->showScreen(page,true));navigation[i]=tab;bar.addView(tab,new LinearLayout.LayoutParams(0,dp(48),1));
        }root.addView(bar);setContentView(root);
        if(saved!=null) {
            loadedPort=saved.getInt("port",-1);loadedBoot=saved.getString("boot","");pendingSection=saved.getString("section","");
            if(loadedPort==RuntimeState.prefs(this).getInt("admin_port",0)) {Bundle web=saved.getBundle("web");if(web!=null)panel.restoreState(web);}else loadedPort=-1;
        }
        showScreen(saved==null?0:saved.getInt("screen",0),false);
        render(null,new JSONObject());root.requestApplyInsets();
        if(Build.VERSION.SDK_INT>=33 && checkSelfPermission(Manifest.permission.POST_NOTIFICATIONS)!=android.content.pm.PackageManager.PERMISSION_GRANTED)
            requestPermissions(new String[]{Manifest.permission.POST_NOTIFICATIONS},52);
    }

    private View buildHome() {
        LinearLayout content=column();heading(content,"Tu red, desde el móvil");
        TextView intro=text("Publica servicios y administra tu Gateway desde aquí.",15,MUTED);content.addView(intro);space(content,22);
        LinearLayout gateway=card(content);gatewayTitle=heading(gateway,"Gateway detenido");
        gatewayDetail=text(getString(R.string.stopped),15,MUTED);gateway.addView(gatewayDetail);space(gateway,12);
        nodeInfo=text("",14,ACCENT);nodeInfo.setTextIsSelectable(true);gateway.addView(nodeInfo);
        gatewayButton=button(gateway,getString(R.string.start),true,()-> {
            if(GatewayService.running || RuntimeState.prefs(this).getBoolean("starting",false))stopGateway();else startGateway();
        });gatewayButton.setId(R.id.gateway_toggle);
        button(gateway,"Abrir administración",true,()->openSection("overview"));
        LinearLayout phone=card(content);heading(phone,"VPN del teléfono");
        vpnDetail=text("Conecta las demás aplicaciones a la misma red del Gateway.",15,MUTED);phone.addView(vpnDetail);
        vpnButton=button(phone,"Conectar aplicaciones",false,()-> {
            if(phoneConnected || phoneStarting)stopService(new Intent(this,GatewayVpnService.class));else requestVpn();
        });vpnButton.setId(R.id.vpn_toggle);
        TextView vpnHelp=text("Puedes mantener los servicios publicados con la VPN del teléfono desconectada.",13,MUTED);space(phone,10);phone.addView(vpnHelp);
        LinearLayout quick=card(content);heading(quick,"Administración");
        button(quick,"Servicios y publicación",false,()->openSection("services"));
        button(quick,"Red y nodos",false,()->openSection("network"));
        button(quick,"Configurar Gateway",false,()->openSection("settings"));
        button(quick,"Copias de seguridad",false,()->openSection("backups"));
        errorText=text("",14,0xffffbf87);content.addView(errorText);return scroll(content);
    }

    private View buildHelp() {
        LinearLayout content=column();heading(content,"Ayuda y acceso");space(content,18);
        LinearLayout common=card(content);heading(common,"Un único Gateway");
        common.addView(text("El panel, los servicios, la configuración y las copias son los mismos que en Linux y PC. Android adapta la pantalla, el almacenamiento y la conexión del teléfono.",15,MUTED));
        button(common,"Configuración del Gateway",false,()->openSection("settings"));
        button(common,"Registros y diagnóstico",false,()->openSection("logs"));
        LinearLayout access=card(content);heading(access,"Tu cuenta administradora");
        access.addView(text("La aplicación abre una sesión normal del panel. Si cambias la contraseña o el método de acceso, utiliza tu cuenta actual.",15,MUTED));
        button(access,getString(R.string.access_data),false,this::showAccess);
        button(access,"Acceso e identidad",false,()->openSection("auth"));
        LinearLayout dns=card(content);dnsHelp=dns;heading(dns,"Revisa el DNS privado");
        dns.addView(text(getString(R.string.private_dns_warning),15,MUTED));button(dns,getString(R.string.private_dns_settings),false,this::openDNSSettings);
        LinearLayout androidCard=card(content);heading(androidCard,"Funcionamiento en Android");
        androidCard.addView(text("Android permite una VPN activa por perfil. Al conectar el teléfono puedes sustituir otra VPN. Las restricciones de batería pueden detener el Gateway; un móvil no garantiza la disponibilidad de un servidor permanente.",15,MUTED));
        button(androidCard,"Abrir ajustes de la aplicación",false,()->startActivity(new Intent(android.provider.Settings.ACTION_APPLICATION_DETAILS_SETTINGS,Uri.parse("package:"+getPackageName()))));
        LinearLayout about=card(content);heading(about,"Headscale Gateway");
        String version="";try {version=getPackageManager().getPackageInfo(getPackageName(),0).versionName;}catch(Exception ignored){}
        about.addView(text("Versión "+version+"\nMotor Go compartido · AGPL-3.0-only",14,MUTED));
        button(about,"Privacidad y datos",false,()->startActivity(new Intent(Intent.ACTION_VIEW,Uri.parse("https://gente-del-condado.github.io/headscale-gateway_ynh/PRIVACIDAD_ANDROID.html"))));
        button(about,getString(R.string.licenses),false,()-> {
            try {new AlertDialog.Builder(this).setTitle(R.string.licenses).setMessage(RuntimeState.read(getAssets().open("avisos.txt"))).setPositiveButton(R.string.accept,null).show();}
            catch(Exception e) {showError(e);}
        });return scroll(content);
    }

    private FrameLayout buildPanel() {
        FrameLayout frame=new FrameLayout(this);panel=new WebView(this);panel.setId(R.id.panel_webview);panel.setBackgroundColor(BG);
        panel.getSettings().setJavaScriptEnabled(true);panel.getSettings().setDomStorageEnabled(true);panel.getSettings().setAllowFileAccess(false);
        panel.getSettings().setMixedContentMode(WebSettings.MIXED_CONTENT_NEVER_ALLOW);
        panel.setWebViewClient(new WebViewClient() {
            @Override public WebResourceResponse shouldInterceptRequest(WebView view,WebResourceRequest request) {
                // Hoja local con el mismo origen: se conserva style-src 'self'.
                if(request.getUrl().toString().equals(RuntimeState.base(MainActivity.this)+"/admin/android-container.css"))try {
                    return new WebResourceResponse("text/css","UTF-8",200,"OK",java.util.Collections.singletonMap("Cache-Control","no-store"),getAssets().open("mobile-panel.css"));
                }catch(Exception ignored){}
                return super.shouldInterceptRequest(view,request);
            }
            @Override public boolean shouldOverrideUrlLoading(WebView view,WebResourceRequest request) {
                Uri url=request.getUrl();if(url.toString().startsWith(RuntimeState.base(MainActivity.this)+"/"))return false;
                if(request.isForMainFrame() && "https".equals(url.getScheme()))try {startActivity(new Intent(Intent.ACTION_VIEW,url));}catch(Exception e){showError(e);}
                return true;
            }
            @Override public void onPageStarted(WebView view,String url,android.graphics.Bitmap icon) {downloads.trust(null);loading.setVisibility(View.VISIBLE);}
            @Override public void onPageFinished(WebView view,String url) {
                if(isPanelURL(url)) {downloads.trust(url);applyMobilePanel();panelPlaceholder.setVisibility(View.GONE);}
                loading.setVisibility(View.GONE);
            }
            @Override public void onReceivedError(WebView view,WebResourceRequest request,WebResourceError error) {
                if(request.isForMainFrame()) {placeholderText.setText("No se ha podido abrir el panel. Comprueba que el Gateway sigue iniciado y vuelve a intentarlo.");panelPlaceholder.setVisibility(View.VISIBLE);}
            }
            @Override public void onReceivedSslError(WebView view,SslErrorHandler handler,android.net.http.SslError error) {handler.cancel();}
        });
        panel.setWebChromeClient(new WebChromeClient() {
            @Override public boolean onJsAlert(WebView view,String url,String message,JsResult result) {
                if(!isPanelURL(url)){result.cancel();return true;}
                new AlertDialog.Builder(MainActivity.this).setTitle("Headscale Gateway").setMessage(message)
                    .setPositiveButton("Aceptar",(d,w)->result.confirm()).setOnCancelListener(d->result.cancel()).show();return true;
            }
            @Override public boolean onJsConfirm(WebView view,String url,String message,JsResult result) {
                if(!isPanelURL(url)){result.cancel();return true;}
                new AlertDialog.Builder(MainActivity.this).setTitle("Confirmar operación").setMessage(message)
                    .setPositiveButton("Continuar",(d,w)->result.confirm()).setNegativeButton("Cancelar",(d,w)->result.cancel())
                    .setOnCancelListener(d->result.cancel()).show();return true;
            }
            @Override public boolean onJsPrompt(WebView view,String url,String message,String initial,JsPromptResult result) {
                if(!isPanelURL(url)){result.cancel();return true;}
                EditText input=new EditText(MainActivity.this);input.setText(initial);
                new AlertDialog.Builder(MainActivity.this).setTitle("Headscale Gateway").setMessage(message).setView(input)
                    .setPositiveButton("Continuar",(d,w)->result.confirm(input.getText().toString()))
                    .setNegativeButton("Cancelar",(d,w)->result.cancel()).setOnCancelListener(d->result.cancel()).show();return true;
            }
            @Override public void onProgressChanged(WebView view,int progress) {loading.setProgress(progress);loading.setVisibility(progress<100?View.VISIBLE:View.GONE);}
            @Override public boolean onShowFileChooser(WebView view,ValueCallback<Uri[]> callback,FileChooserParams params) {
                if(fileResult!=null)fileResult.onReceiveValue(null);fileResult=callback;
                startActivityForResult(Intent.createChooser(new Intent(Intent.ACTION_OPEN_DOCUMENT).setType("*/*").addCategory(Intent.CATEGORY_OPENABLE),getString(R.string.file_choose)),51);return true;
            }
        });
        CookieManager.getInstance().setAcceptThirdPartyCookies(panel,false);downloads=new PanelDownloads(this,panel);
        panel.addJavascriptInterface(downloads,"DescargasGateway");panel.setDownloadListener((url,agent,disposition,type,length)->downloads.begin(url,disposition,type));
        frame.addView(panel,new FrameLayout.LayoutParams(-1,-1));
        LinearLayout empty=column();empty.setPadding(dp(26),dp(30),dp(26),dp(30));empty.setBackgroundColor(BG);empty.setGravity(Gravity.CENTER_VERTICAL);
        heading(empty,"Administración del Gateway");placeholderText=text("Inicia el Gateway para acceder al mismo panel que utilizas en tu PC.",16,MUTED);space(empty,14);empty.addView(placeholderText);
        button(empty,"Abrir panel",true,()-> {loadedPort=-1;if(!GatewayService.running)startGateway();});
        panelPlaceholder=scroll(empty);panelPlaceholder.setId(R.id.panel_placeholder);frame.addView(panelPlaceholder,new FrameLayout.LayoutParams(-1,-1));
        loading=new ProgressBar(this,null,android.R.attr.progressBarStyleHorizontal);loading.setMax(100);loading.setVisibility(View.GONE);
        frame.addView(loading,new FrameLayout.LayoutParams(-1,dp(3),Gravity.TOP));return frame;
    }

    private boolean isPanelURL(String url) {return url!=null && url.startsWith(RuntimeState.base(this)+"/admin/");}
    private void applyMobilePanel() {
        if(!isPanelURL(panel.getUrl()))return;
        String code="if(location.origin==="+JSONObject.quote(RuntimeState.base(this))+" && location.pathname.startsWith('/admin/')){let s=document.getElementById('android-style');if(!s){s=document.createElement('link');s.rel='stylesheet';s.id='android-style';s.href='/admin/android-container.css';document.head.append(s);}"+mobileJS+";";
        if(!pendingSection.isEmpty())code+="window.gatewayAndroidOpenSection("+JSONObject.quote(pendingSection)+");";
        panel.evaluateJavascript(code+"}",null);pendingSection="";
    }
    private void openSection(String section) {pendingSection=section;showScreen(1,true);if(isPanelURL(panel.getUrl()))applyMobilePanel();}
    private void showScreen(int page,boolean startIfNeeded) {
        if(clearBackCallback!=null){clearBackCallback.run();clearBackCallback=null;}
        if(Build.VERSION.SDK_INT>=33 && page!=0)clearBackCallback=BackGesture.register(this,()->showScreen(0,false));
        screen=page;home.setVisibility(page==0?View.VISIBLE:View.GONE);panelPage.setVisibility(page==1?View.VISIBLE:View.GONE);help.setVisibility(page==2?View.VISIBLE:View.GONE);
        sectionsButton.setVisibility(page==1?View.VISIBLE:View.GONE);headerCaption.setText(page==1?"Administración":page==2?"Acceso, ayuda y aplicación":"Tu Gateway, también en Android");
        for(int i=0;i<3;i++) {navigation[i].setTextColor(i==page?ACCENT:MUTED);navigation[i].setBackground(shape(i==page?SURFACE:0xff0b1220,12));navigation[i].setSelected(i==page);}
        if(page==1 && startIfNeeded && !GatewayService.running && !RuntimeState.prefs(this).getBoolean("starting",false))startGateway();
    }
    private void showSections() {
        if(!isPanelURL(panel.getUrl())) {Toast.makeText(this,"Espera a que se abra el panel.",Toast.LENGTH_SHORT).show();return;}
        panel.evaluateJavascript("JSON.stringify(window.gatewayAndroidSections?window.gatewayAndroidSections():[])",raw-> {
            try {
                JSONArray entries=new JSONArray(new JSONTokener(raw).nextValue().toString());
                if(entries.length()==0) {Toast.makeText(this,"Inicia sesión en el panel para ver sus secciones.",Toast.LENGTH_SHORT).show();return;}
                PopupMenu menu=new PopupMenu(this,sectionsButton);
                for(int i=0;i<entries.length();i++) {
                    JSONObject entry=entries.getJSONObject(i);String section=entry.getString("id");
                    menu.getMenu().add(entry.getString("title")).setOnMenuItemClickListener(item->{openSection(section);return true;});
                }
                menu.getMenu().add("Cerrar sesión del panel").setOnMenuItemClickListener(item->{panel.evaluateJavascript("document.getElementById('logout').click()",null);return true;});menu.show();
            }catch(Exception e){showError(e);}
        });
    }
    private void startGateway() {
        RuntimeState.prefs(this).edit().putBoolean("desired_running",true).remove("last_error").commit();
        Intent intent=new Intent(this,GatewayService.class);if(Build.VERSION.SDK_INT>=26)startForegroundService(intent);else startService(intent);
        loadedPort=-1;gatewayButton.setEnabled(false);gatewayDetail.setText(R.string.opening);placeholderText.setText(R.string.opening);
    }
    private void stopGateway() {
        RuntimeState.prefs(this).edit().putBoolean("desired_running",false).commit();stopService(new Intent(this,GatewayVpnService.class));stopService(new Intent(this,GatewayService.class));
        loadedPort=-1;engineReady=false;nodeConnected=false;panel.loadUrl("about:blank");panelPlaceholder.setVisibility(View.VISIBLE);render(null,new JSONObject());
    }
    private void requestVpn() {
        new AlertDialog.Builder(this).setTitle("Conectar las aplicaciones a tu red privada")
            .setMessage("El adaptador procesa las conexiones TCP, UDP y DNS de las aplicaciones. El tráfico de tu red privada va cifrado a sus nodos. Internet usa la conexión habitual del teléfono, salvo que configures un nodo de salida; en ese caso pasa por él. Android solo permite una VPN activa por perfil.\n\nLa app no incorpora publicidad ni analítica. Tu servidor y los proveedores que elijas pueden registrar metadatos según su configuración. Puedes desconectar la VPN desde esta app o desde Android.")
            .setNegativeButton(android.R.string.cancel,null)
            .setPositiveButton("Continuar",(dialog,which)->requestSystemVpn()).show();
    }
    private void requestSystemVpn() {
        Intent consent=android.net.VpnService.prepare(this);if(consent!=null)startActivityForResult(consent,50);else startVpn();
    }
    private void startVpn() {
        worker.execute(()-> {
            try {
                if(!RuntimeState.control(this,"/state").optBoolean("connected"))throw new IllegalStateException(getString(R.string.vpn_pending));
                post(()-> {Intent start=new Intent(this,GatewayVpnService.class);if(Build.VERSION.SDK_INT>=26)startForegroundService(start);else startService(start);});
            }catch(Exception e){showError(e);}
        });
    }
    private void render(JSONObject state,JSONObject phone) {
        boolean starting=RuntimeState.prefs(this).getBoolean("starting",false);engineReady=state!=null;nodeConnected=engineReady&&state.optBoolean("connected");
        boolean running=GatewayService.running||starting;
        phoneConnected="connected".equals(phone.optString("phase"));phoneStarting="starting".equals(phone.optString("phase"));
        gatewayTitle.setText(nodeConnected?"Conectado a Headscale":starting?"Iniciando Gateway":engineReady?"Gateway iniciado":running?"Gateway en ejecución":"Gateway detenido");
        gatewayTitle.setTextColor(nodeConnected?ACCENT:TEXT);
        gatewayDetail.setText(nodeConnected?"Tu nodo está conectado. Administra y publica tus servicios desde el panel.":starting?getString(R.string.opening):engineReady?"El panel está disponible. Configura Headscale o revisa la conexión del nodo.":running?"Esperando respuesta del motor. Si continúa, revisa los registros del Gateway.":getString(R.string.stopped));
        String ips="";if(state!=null && state.optJSONArray("ips")!=null) {JSONArray a=state.optJSONArray("ips");for(int i=0;i<a.length();i++)ips+=(i==0?"":"\n")+a.optString(i);}
        nodeInfo.setText(engineReady?state.optInt("services")+" servicios activos"+(ips.isEmpty()?"":"\n"+ips):"");nodeInfo.setVisibility(engineReady?View.VISIBLE:View.GONE);
        gatewayButton.setEnabled(true);gatewayButton.setText(running?getString(R.string.stop):getString(R.string.start));
        gatewayButton.setTextColor(running?TEXT:BG);gatewayButton.setBackground(shape(running?SURFACE:ACCENT,12));
        vpnButton.setEnabled(nodeConnected||phoneConnected||phoneStarting);vpnButton.setAlpha(vpnButton.isEnabled()?1f:.5f);
        vpnButton.setText(phoneConnected||phoneStarting?"Desconectar aplicaciones":"Conectar aplicaciones");
        vpnDetail.setText(phoneConnected?"Las aplicaciones del teléfono utilizan la conexión del Gateway.":phoneStarting?"Preparando la conexión de las aplicaciones…":nodeConnected?"Gateway conectado. Puedes conectar también las aplicaciones del teléfono.":"Conecta primero el nodo del Gateway a Headscale desde el panel.");
        String error=RuntimeState.prefs(this).getString("last_error","");if(!phone.optString("error").isEmpty())error=phone.optString("error");errorText.setText(error);errorText.setVisibility(error.isEmpty()?View.GONE:View.VISIBLE);
        if(!engineReady && !starting)placeholderText.setText("El panel estará disponible cuando el Gateway esté iniciado. Pulsa Abrir panel para continuar.");
        if(!running && !engineReady)panelPlaceholder.setVisibility(View.VISIBLE);
    }
    private void showError(Exception e) {post(()->Toast.makeText(this,getString(R.string.error,e.getMessage()),Toast.LENGTH_LONG).show());}
    private void showAccess() {
        worker.execute(()-> {try {String password=RuntimeState.secret(this,"admin_password");post(()->new AlertDialog.Builder(this).setTitle(R.string.access_data).setMessage(getString(R.string.access_info,password)).setPositiveButton(R.string.accept,null).show());}catch(Exception e){showError(e);}});
    }
    private void openDNSSettings() {
        try {startActivity(new Intent("android.settings.PRIVATE_DNS_SETTINGS"));}catch(Exception e){startActivity(new Intent(android.provider.Settings.ACTION_WIRELESS_SETTINGS));}
    }
    void openCertificateSettings() {
        try {startActivity(new Intent(android.provider.Settings.ACTION_SECURITY_SETTINGS));}
        catch(Exception e){showError(e);}
    }
    private void openPanel() throws Exception {
        String base=RuntimeState.base(this);HttpURLConnection connection=(HttpURLConnection)new URL(base+"/api/v1/auth/login").openConnection();
        connection.setRequestMethod("POST");connection.setConnectTimeout(3000);connection.setReadTimeout(6000);connection.setInstanceFollowRedirects(false);
        connection.setRequestProperty("Content-Type","application/json");connection.setRequestProperty("Origin",base);connection.setDoOutput(true);
        byte[] body=new JSONObject().put("username","android").put("password",RuntimeState.secret(this,"admin_password")).toString().getBytes(StandardCharsets.UTF_8);
        String loginCookie=null;
        try {try(java.io.OutputStream out=connection.getOutputStream()){out.write(body);}if(connection.getResponseCode()==200)loginCookie=connection.getHeaderField("Set-Cookie");}finally{connection.disconnect();}
        final String cookie=loginCookie;post(()-> {
            if(cookie==null){panel.loadUrl(base+"/admin/");return;}
            CookieManager.getInstance().setCookie(base,cookie,accepted->{if(!destroyed){CookieManager.getInstance().flush();panel.loadUrl(base+"/admin/");}});
        });
    }
    private final Runnable poll=new Runnable(){@Override public void run(){
        if(!visible||destroyed)return;
        if(!checking){checking=true;worker.execute(()-> {
            try {
                JSONObject phone=RuntimeState.phoneVPN(MainActivity.this);
                if(RuntimeState.prefs(MainActivity.this).getBoolean("starting",false)){post(()->render(null,phone));return;}
                JSONObject state=RuntimeState.control(MainActivity.this,"/state");post(()->render(state,phone));
                int port=RuntimeState.prefs(MainActivity.this).getInt("admin_port",0);String boot=state.optString("boot_id","");
                if(loadedPort!=port||!loadedBoot.equals(boot)){openPanel();loadedPort=port;loadedBoot=boot;}
            }catch(Exception e){JSONObject phone=RuntimeState.phoneVPN(MainActivity.this);post(()->render(null,phone));}
            finally{checking=false;}
        });}ui.postDelayed(this,2000);
    }};
    @Override protected void onResume(){super.onResume();visible=true;boolean strict=false;
        if(Build.VERSION.SDK_INT>=28)try{strict="hostname".equals(android.provider.Settings.Global.getString(getContentResolver(),"private_dns_mode"));}catch(SecurityException ignored){}
        dnsHelp.setVisibility(strict?View.VISIBLE:View.GONE);downloads.offerPending();ui.post(poll);
    }
    @Override protected void onPause(){visible=false;ui.removeCallbacks(poll);super.onPause();}
    @Override protected void onSaveInstanceState(Bundle out){super.onSaveInstanceState(out);out.putInt("screen",screen);out.putInt("port",loadedPort);out.putString("boot",loadedBoot);out.putString("section",pendingSection);Bundle web=new Bundle();panel.saveState(web);out.putBundle("web",web);}
    @Override protected void onDestroy(){if(clearBackCallback!=null)clearBackCallback.run();destroyed=true;ui.removeCallbacks(poll);worker.shutdownNow();if(fileResult!=null)fileResult.onReceiveValue(null);downloads.detach();panel.destroy();super.onDestroy();}
    @Override protected void onActivityResult(int code,int result,Intent data){super.onActivityResult(code,result,data);
        if(code==50&&result==RESULT_OK)startVpn();
        if(code==51&&fileResult!=null){Uri chosen=result==RESULT_OK&&data!=null?data.getData():null;fileResult.onReceiveValue(chosen!=null&&"content".equals(chosen.getScheme())?new Uri[]{chosen}:null);fileResult=null;}
        if(code==53){Uri destination=result==RESULT_OK&&data!=null?data.getData():null;worker.execute(()->downloads.save(destination));}
    }
    // El callback moderno solo intercepta Atrás fuera de Inicio; la raíz conserva el gesto del sistema.
    @android.annotation.TargetApi(33)
    private static final class BackGesture {
        static Runnable register(Activity activity,Runnable action) {
            android.window.OnBackInvokedCallback callback=action::run;
            activity.getOnBackInvokedDispatcher().registerOnBackInvokedCallback(android.window.OnBackInvokedDispatcher.PRIORITY_DEFAULT,callback);
            return ()->activity.getOnBackInvokedDispatcher().unregisterOnBackInvokedCallback(callback);
        }
    }
    // Compatibilidad para API 24–32; las versiones nuevas usan BackGesture.
    @android.annotation.SuppressLint("GestureBackNavigation")
    @Override public void onBackPressed(){if(screen!=0){showScreen(0,false);return;}super.onBackPressed();}
}
