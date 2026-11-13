'use strict';
// Experiencia del panel: ayudas accesibles, avisos, reinicio pendiente y estado.
// No se almacenan contraseñas, tokens, ni valores de formularios en el navegador.
(() => {
  // Propiedades añadidas por el Gateway al objeto Window (no son parte del DOM).
  const adminWindow = /** @type {Window & typeof globalThis & {
   *   toastMessage: (message: string, error?: boolean) => void,
   *   _gatewayOfflineToast?: boolean,
   *   proxySchema?: Array<{ key: string, label?: string, description?: string }>,
   *   configSchema?: Array<{ key: string, label?: string, description?: string }>,
   *   markRestartRequired: () => void,
   *   notifyRestartRequested: () => void,
   *   renderStatus?: (status: any) => void
   * }} */ (window);
  const byId = id => document.getElementById(id);
  adminWindow.toastMessage = (message, error=false) => {
    const stack=byId('toast-stack');if(!stack)return;
    const toast=document.createElement('div');toast.className='toast'+(error?' toast-error':'');
    const msg=document.createElement('span');msg.textContent=message;
    const close=document.createElement('button');close.type='button';close.textContent='×';close.className='toast-close';close.setAttribute('aria-label','Cerrar aviso');
    close.addEventListener('click',()=>toast.remove());toast.append(msg,close);stack.prepend(toast);
    while(stack.children.length>4)stack.lastElementChild.remove();
    const clear=()=>{if(toast.isConnected)toast.remove()};
    setTimeout(clear,error?12000:5500);
  };
  const helpById={
    'service-name':'Subdominio publicado. Solo letras minúsculas, números y guiones; cambiar el nombre requiere crear otro servicio.',
    backend:'URL a la que el Gateway envía las peticiones. En Home Assistant remoto: http://100.64.0.8:8123.',
    template:'Selecciona una plantilla guardada en SQLite. Los campos no indicados en el servicio se heredan.',
    enabled:'Desactivado conserva el servicio en el catálogo, sin publicarlo ni crear registros DNS.',
    'service-description':'Texto visible en el catálogo y en el portal público.',
    'network-url':'Dirección HTTPS del servidor Headscale, usada para consultar la API y registrar el nodo integrado.',
    'network-key':'Clave administrativa de Headscale. Dejar vacío conserva la que está cifrada en SQLite; nunca aparece en la API.',
    'network-enabled':'Muestra u oculta las funciones de inventario y diagnóstico. Requiere reiniciar.',
    'network-search':'Filtra el listado de nodos por nombre, IP, usuario o etiquetas. No cambia los dispositivos.',
    'network-ping-type':'DISCO comprueba conectividad Tailscale; TSMP comprueba el túnel y ICMP comprueba la pila IP.',
    'auth-user':'Usuario habilitado por el método local o LDAP configurado.',
    'auth-password':'Contraseña de esa cuenta. El navegador puede gestionarla con su gestor de contraseñas; Gateway no la conserva en cookies.',
    'auth-totp':'Código TOTP de seis dígitos. Solo se solicita si 2FA está activado para la cuenta local.',
    token:'Token de recuperación administrativo. No se guarda en cookies ni localStorage.',
    'mfa-code':'Código temporal de seis dígitos generado por el autenticador para confirmar el alta del segundo factor.',
    'api-token-days':'Número de días hasta que caduque esta clave API delegada. Debe rotarse antes de caducar.',
    'api-token-role':'Rol de la clave: viewer solo lectura; operator habilita algunas operaciones según scopes.',
    'api-token-label':'Descripción administrativa de la integración que utilizará la clave.',
    'template-json':'Las plantillas guardan opciones de proxy en JSON dentro de SQLite; egress_mode puede ser system o tailnet.',
    'template-category':'Categoría que aparecerá en el catálogo de plantillas.',
    'template-description':'Ayuda descriptiva para identificar cuándo conviene utilizar esta plantilla.',
    'template-backend-hint':'Ejemplo de backend HTTP(S). No configura ningún servicio ni abre puertos.',
    'auth-mode':'Elige token, cuenta local, LDAP o Dex/OIDC. El token de recuperación permanece disponible.',
    'auth-local-password':'La contraseña local no se recupera ni se almacena en el navegador. Vacío: conserva la actual.',
    'remember-session':'Crea una cookie HttpOnly de sesión durante 30 días y guarda una sesión revocable, cifrada en SQLite. Nunca almacena la contraseña.',
    'auth-ldap-url':'Utiliza LDAPS con verificación TLS. Recomendada la integración Dex para accesos externos.',
    'auth-oidc-issuer':'URL issuer exacta del proveedor, coincidente con su documento de descubrimiento OIDC.',
    'auth-oidc-redirect':'Debe coincidir exactamente con la URL autorizada por el proveedor OIDC, con HTTPS.',
    'auth-role-emails':'Mapea únicamente correos verificados y expresamente autorizados: correo=admin, operator o viewer.',
    'auth-oidc-disable-groups':'Activa para Google y proveedores sin claim groups. Si utilizas Dex, conserva este permiso desactivado.',
    'auth-role-groups':'Una asignación por línea grupo=rol; no se concede rol admin por el mero hecho de autenticarse.',
    'api-token-scopes':'Concede solo los permisos necesarios para la integración. Nunca compartas el token de recuperación.',
    'backup-interval':'Programa copias mientras el Gateway esté arrancado; 0 desactiva las automáticas.',
    'backup-keep':'Cuántos juegos completos de copia local se conservan. El resto se elimina al generar una nueva copia.',
    'backup-include-acl':'Si Headscale responde, añade la política ACL y los registros DNS como instantáneas. No incluye el YAML del servidor VPS.',
    'log-follow':'Actualiza automáticamente los tres archivos cada 3 segundos mientras la pestaña está abierta.',
    'log-max-mb':'Tamaño máximo de un archivo de log antes de rotarlo.',
    'proxy.default_egress':'Sistema utiliza net.Dialer y las rutas/tailscaled de Fedora. Tailnet utiliza la identidad propia tsnet del Gateway. Cambiar el ajuste global requiere reiniciar.'
  };
  function installHelp(target,description) {
    if (!target || !description || target.querySelector(':scope > .help-trigger')) return;
    const button=document.createElement('button');button.className='help-trigger';button.type='button';button.textContent='?';button.setAttribute('aria-label','Ayuda: '+description);
    button.setAttribute('aria-expanded','false');
    const bubble=document.createElement('span');bubble.className='help-bubble';bubble.setAttribute('role','tooltip');bubble.textContent=description;bubble.hidden=true;
    button.addEventListener('click',ev=>{ev.preventDefault();ev.stopPropagation();const open=bubble.hidden;document.querySelectorAll('.help-bubble').forEach(n=>{if(n instanceof HTMLElement)n.hidden=true;});document.querySelectorAll('.help-trigger').forEach(n=>n.setAttribute('aria-expanded','false'));bubble.hidden=!open;button.setAttribute('aria-expanded',String(open));});
    target.classList.add('help-target');target.append(button,bubble);
  }
  function installStaticHelp() {
    for(const [id,help] of Object.entries(helpById)){
      const input=byId(id);if(!input)continue;
      const label=input.closest('label');if(label)installHelp(label,help);
    }
  }
  // Los renderizadores reconstruyen campos según el esquema. Observa solo los
  // contenedores de configuración y proxy, no todo el documento.
  function observeHelp(id){const container=byId(id);if(!container)return;
    const decorate=()=>container.querySelectorAll('label.config-field').forEach(label=>{
      const field=label.querySelector('[data-proxy-key],[data-key]');
      if(!(field instanceof HTMLElement))return;
      const schema=(field.dataset.proxyKey ? adminWindow.proxySchema : adminWindow.configSchema)||[];
      const key=field.dataset.proxyKey||field.dataset.key;
      const item=schema.find(s=>s.key===key);
      installHelp(label,item?.description || window.tr("Ajuste «{p0}». Los campos opcionales del servicio pueden heredar la plantilla; consulta su valor antes de modificarlo.", {p0:((item?.label||key))}));
    });
    new MutationObserver(decorate).observe(container,{childList:true,subtree:false});
    // Los campos proxy se insertan en fieldsets anidados: el observador detecta
    // los nuevos y la función diferida añade sus ayudas.
    new MutationObserver(decorate).observe(container,{childList:true,subtree:true});
    decorate();
  }
  // La sesión del navegador solo guarda la marca de reinicio pendiente, nunca secretos.
  const pendingKey='hg.restart.pending',bootKey='hg.last.boot';
  function pending(){try{return sessionStorage.getItem(pendingKey)==='1'}catch{return false}}
  function showPending(){byId('restart-banner').hidden=!pending()}
  adminWindow.markRestartRequired=()=>{try{sessionStorage.setItem(pendingKey,'1')}catch{}showPending();};
  adminWindow.notifyRestartRequested=()=>{adminWindow.toastMessage('Reiniciando… comprobando disponibilidad del panel.');};
  function markBoot(status){
    const boot=status.boot_id;if(!boot)return;
    try{
      const last=sessionStorage.getItem(bootKey);
      if(last && last!==boot && pending()){
        sessionStorage.removeItem(pendingKey);showPending();adminWindow.toastMessage('Gateway reiniciado: los ajustes guardados ya pueden estar activos.');
      }
      sessionStorage.setItem(bootKey,boot);
    }catch{}
  }
  byId('restart-banner-action').addEventListener('click',()=>byId('restart-gateway').click());
  byId('restart-banner-dismiss').addEventListener('click',()=>{try{sessionStorage.removeItem(pendingKey)}catch{}showPending()});
  document.querySelectorAll('[data-help]').forEach(node=>{if(!(node instanceof HTMLElement))return;node.title=node.dataset.help||'';node.addEventListener('click',()=>adminWindow.toastMessage(node.dataset.help||''));});
  installStaticHelp();observeHelp('proxy-fields');observeHelp('config-fields');showPending();
  let checking=false;
  async function updateLive(){
    if(checking || document.hidden || byId('app').hidden)return;
    checking=true;
    try{
      const status=await api('status');
      renderStatus(status);markBoot(status);
      byId('connection').textContent=window.tr("Sesión: {p0} · conectado", {p0:(currentRole)});
    }catch(e){
      if(e?.status===401){
        byId('app').hidden=true;byId('login').hidden=false;
        byId('connection').textContent='Sesión caducada';
        adminWindow.toastMessage('La sesión ha caducado. Vuelve a iniciar sesión.',true);
        if(typeof getAuthMethods==='function')void getAuthMethods();
        return;
      }
      byId('connection').textContent='Panel sin conexión';
      if(!adminWindow._gatewayOfflineToast){adminWindow.toastMessage('No se pudo contactar con el Gateway; se volverá a intentar.',true);adminWindow._gatewayOfflineToast=true;}
    }finally{checking=false}
  }
  // El intervalo de 8 segundos detecta reinicios sin recargar ni descartar el
  // formulario que se edita. No añade trabajo cuando la pestaña está oculta.
  setInterval(updateLive,8000);
  document.addEventListener('visibilitychange',()=>{if(!document.hidden)void updateLive()});
  // También detecta el identificador de arranque al consultar el estado, sin modificarlo.
  const originalRender=adminWindow.renderStatus;
  if(typeof originalRender==='function'){
    adminWindow.renderStatus=function(data){originalRender(data);markBoot(data);adminWindow._gatewayOfflineToast=false;};
  }
})();

// Las preferencias de navegación no contienen credenciales ni configuración funcional.
(() => {
  const nav=document.querySelector('.main-navigation');
  const label=document.createElement('label');label.className='nav-search-label';label.textContent='BUSCAR SECCIÓN · /';
  const search=document.createElement('input');search.type='search';search.placeholder='Nodos, certificados, logs…';search.setAttribute('aria-label','Buscar sección del panel');label.append(search);nav.prepend(label);
  const heading=document.createElement('div');heading.className='workspace-heading';
  const text=document.createElement('div'),eyebrow=document.createElement('small'),title=document.createElement('h1'),description=document.createElement('p');
  eyebrow.textContent='CENTRO DE ADMINISTRACIÓN';text.append(eyebrow,title,description);
  const density=document.createElement('button');density.type='button';density.className='secondary';density.textContent='Vista compacta';density.setAttribute('aria-pressed','false');heading.append(text,density);nav.after(heading);
  const descriptions={'headscale-users':'Usuarios de Headscale y claves para registrar dispositivos. Los grupos de acceso se editan en Reglas ACL.','policy-templates':'Plantillas ACL para preparar un borrador antes de aplicarlo.',overview:'Estado del Gateway y accesos a las tareas habituales.',services:'Publica y administra tus servicios internos.',network:'Encuentra equipos, revisa sus rutas y comprueba la conectividad.',pki:'Gestiona autoridades, certificados y confianza del Gateway y del sistema.', 'policy-dns':'Define quién puede acceder a cada equipo y servicio. Los cambios se preparan como borrador.',logs:'Busca incidencias con la hora de tu equipo y controla los registros.',settings:'Configura el Gateway. Los cambios que lo necesiten se aplican al reiniciar.',backups:'Crea, verifica y recupera copias cifradas de tu instalación.'};
  function updateHeading(){const button=nav.querySelector('[data-tab].selected');if(!button)return;title.textContent=button.textContent;description.textContent=descriptions[button.dataset.tab]||'Configuración y herramientas de administración.';nav.querySelectorAll('[data-tab]').forEach(b=>{if(b===button)b.setAttribute('aria-current','page');else b.removeAttribute('aria-current');});}
  new MutationObserver(updateHeading).observe(nav,{subtree:true,attributes:true,attributeFilter:['class']});updateHeading();
  nav.addEventListener('click',event=>{if(event.target instanceof Element && event.target.closest('[data-tab]'))window.scrollTo({top:0,behavior:'auto'});});
  search.addEventListener('input',()=>{const q=search.value.toLocaleLowerCase();for(const button of nav.querySelectorAll('[data-tab]'))button.style.display=button.textContent.toLocaleLowerCase().includes(q)?'':'none';});
  document.addEventListener('keydown',event=>{if(event.key==='/'&&!event.ctrlKey&&!event.metaKey&&!['INPUT','TEXTAREA','SELECT'].includes(document.activeElement?.tagName)){event.preventDefault();search.focus();}if(event.key==='Escape'&&document.activeElement===search){search.value='';search.dispatchEvent(new Event('input'));search.blur();}});
  const setDensity=compact=>{document.body.classList.toggle('compact',compact);density.setAttribute('aria-pressed',String(compact));density.textContent=compact?'Vista cómoda':'Vista compacta';};
  try{setDensity(localStorage.getItem('hgw.compact')==='true')}catch{}
  density.addEventListener('click',()=>{const compact=!document.body.classList.contains('compact');setDensity(compact);try{localStorage.setItem('hgw.compact',String(compact))}catch{}});
  const shortcuts=document.createElement('div');shortcuts.className='quick-actions';
  for(const [tab,name,help] of [['services','Gestionar servicios','Publicación y proxy HTTPS'],['network','Revisar la red','Equipos, rutas y diagnósticos'],['logs','Consultar registros','Incidencias y actividad reciente']]){const b=document.createElement('button');b.type='button';b.textContent=name;const small=document.createElement('small');small.textContent=help;b.append(small);b.addEventListener('click',()=>nav.querySelector('[data-tab="'+tab+'"]').click());shortcuts.append(b);}
  document.querySelector('[data-page="overview"]').prepend(shortcuts);
  // Filter existing rendered text without re-requesting data or changing records.
  for(const id of ['service-rows','template-rows','pki-local-rows','pki-peer-rows','backup-rows']){
    const body=document.getElementById(id);if(!body)continue;
    const bar=document.createElement('div');bar.className='table-toolbar';const input=document.createElement('input');input.type='search';input.placeholder='Filtrar esta lista…';input.setAttribute('aria-label','Filtrar '+id);const count=document.createElement('span');count.setAttribute('role','status');bar.append(input,count);body.closest('.tablewrap').before(bar);
    const filter=()=>{let visible=0;for(const row of body.rows){row.hidden=!row.textContent.toLocaleLowerCase().includes(input.value.toLocaleLowerCase());if(!row.hidden)visible++;}count.textContent=body.rows.length?`${visible} de ${body.rows.length} registros`:'Todavía no hay registros';};
    input.addEventListener('input',filter);new MutationObserver(filter).observe(body,{childList:true,subtree:true});filter();
  }
})();
