/// <reference path="./panel-globals.d.ts" />
'use strict';

let token = '';
let currentRole = 'admin';
let csrf = '';
let authMethods = {mode:'token'};
let authSettings = null;
let services = [];
let templates = [];
let settings = {};
let configSchema = [];
let proxySchema = [];
let editedProxy = {};
let lastGatewayDomain = "";

const el = id => document.getElementById(id);
const tr = message => (typeof window.tr === 'function' ? window.tr(message) : message);
const notice = (msg, bad = false) => {
  el('notice').textContent = msg;
  el('notice').style.color = bad ? '#fda4af' : '#86efac';
  if (msg && typeof window.toastMessage === 'function') window.toastMessage(msg, bad);
};

async function api(path, method = 'GET', body) {
  const response = await fetch(window.gatewayURL('/api/v1/' + path), {
    method,
    cache: 'no-store',
    headers: {
      ...(token ? {Authorization:'Bearer '+token} : {}),
      ...(csrf ? {'X-CSRF-Token':csrf} : {}),
      'Accept-Language':window.gatewayLanguage?.()||'es',
      ...(body === undefined ? {} : {'Content-Type': 'application/json'})
    },
    body: body === undefined ? undefined : JSON.stringify(body)
  });
  let data;
  try { data = await response.json(); }
  catch { throw Error('Respuesta inválida del servidor'); }
  if (!response.ok) { if(response.status===401){currentRole='';csrf='';token='';el('app').hidden=true;el('login').hidden=false;} const err=Error(data.error || window.tr("Error HTTP {p0}", {p0:(response.status)})); err.status=response.status; throw err; }
  return data;
}

const run = async fn => {
  try { await fn(); }
  catch (e) { notice(e.message, true); }
};

async function refresh() {
  const session=await api('auth/session');
  csrf=session.csrf||csrf;
  el('logout').hidden=session.source==='proxy';
  currentRole=session.role || 'admin';
  const isAdmin=currentRole==='admin';
  const [status,list,tmpl,proxyFields]=await Promise.all([
    api('status'),api('services'),api('templates'),api('proxy/schema')
  ]);
  lastGatewayDomain = status.domain || "";
  services=list;templates=tmpl;proxySchema=proxyFields;
  window.proxySchema=proxySchema;
  if(isAdmin){
    [settings,configSchema]=await Promise.all([api('config'),api('config/schema')]);
    renderSettings();
    window.configSchema=configSchema;
    el('policy-dns-nav').hidden = settings['headscale.panel_enabled'] !== 'true';
    el('magicdns-nav').hidden = settings['headscale.panel_enabled'] !== 'true';
    el('network-url').value=settings['headscale.url'] || '';
    el('network-key').value='';
    el('network-enabled').checked=settings['headscale.panel_enabled']==='true';
    await refreshAuthSettings();
    await loadExposure();
  }
  el('settings-nav').hidden=!isAdmin;
  el('auth-nav').hidden=!isAdmin;
  el('exposure-nav').hidden=!isAdmin;
  el('logs-nav').hidden=!isAdmin;
  el('backups-nav').hidden=!isAdmin;
  el('pki-nav').hidden=!isAdmin;
 el('portal-nav').hidden=!isAdmin;
  for(const id of ['headscale-users-nav','policy-templates-nav'])el(id).hidden=!isAdmin||settings['headscale.panel_enabled']!=='true';
  el('overview-backups').hidden=!isAdmin;
  if(isAdmin&&typeof refreshBackupOverview==='function')await refreshBackupOverview();
  el('backup-panel').hidden=!isAdmin;
  el('restart-gateway').hidden=!isAdmin;
  el('refresh-acme').hidden=!isAdmin;
  el('new-service').hidden=currentRole==='viewer';
  el('reload').hidden=currentRole==='viewer';
  el('new-template').hidden=currentRole==='viewer';
  renderStatus(status);renderServices();
  await initializeNetwork();
  el('connection').textContent=window.tr("Sesión: {p0}", {p0:(currentRole)});
}

function renderStatus(data) {
  el('setup-banner').hidden = !data.setup_required;
  el('setup-message').textContent = data.setup_message || '';
  el('reload').disabled = !!data.setup_required;
  const dl = el('status');
  dl.replaceChildren();
  for (const [key, value] of Object.entries(data)) {
    const dt = document.createElement('dt');
    const labels={lan_https_active:'HTTPS LAN disponible',ca_http_port:'Puerto HTTP de CA activo',subscription:tr('Suscripción'),node:'Nodo',domain:'Dominio',services_active:'Servicios activos',vpn_connected:'VPN conectada',vpn_state:'Estado VPN',certificate_mode:'Modo TLS',certificate_source:'Certificado',dns_provider:'Proveedor DNS',headscale_url:'Servidor Headscale',uptime_seconds:'Tiempo en servicio',boot_id:'Inicio del proceso',data_dir:'Directorio de datos',admin_domain_mode:'Acceso al panel',admin_domain_url:'URL administrativa',admin_oidc_callback:'Retorno OIDC',vpn_identity_confirmed:'Identidad VPN confirmada',setup_required:'Configuración pendiente',setup_message:'Configuración',acme_retry_after_utc:'Próximo intento ACME'};
    dt.textContent = labels[key]||key.replaceAll('_',' ');
    const dd = document.createElement('dd');
    if(key==='subscription') {
      const states={disabled:tr('Módulo desactivado'),active:tr('Activa'),grace:tr('Periodo de gracia'),expired:tr('Caducada'),missing:tr('Pendiente de activación'),configuration_error:tr('Error de configuración'),invalid:tr('Licencia inválida'),not_yet_valid:tr('Todavía no válida'),missing_feature:tr('Función no incluida')};
      dd.textContent=tr(states[value.state]||value.state)+(value.instance_id?' · ID: '+value.instance_id:'')+(value.expires_at?' · '+new Date(value.expires_at*1000).toLocaleString():'');
      dl.append(dt,dd);continue;
    }
    dd.textContent = typeof value==='boolean'?(value?'Sí':'No'):key==='uptime_seconds'?`${Math.floor(Number(value)/3600)} h ${Math.floor(Number(value)%3600/60)} min`:key==='boot_id'?new Date(value).toLocaleString():String(value);
    dl.append(dt, dd);
  }
}

function button(label, callback, kind) {
  const b = document.createElement('button');
  b.type = 'button';
  b.textContent = label;
  if (kind) b.className = kind;
  b.addEventListener('click', callback);
  return b;
}

function renderServices() {
  const rows = el('service-rows');
  rows.replaceChildren();
  services.forEach(item => {
    const tr = document.createElement('tr');
    for (const value of [item.name, item.backend || '—', item.enabled ? 'Activo' : 'Desactivado']) {
      const td = document.createElement('td');
      td.textContent = value;
      tr.append(td);
    }
    if (item.description) tr.querySelector('td').title = item.description;
    const mode = item.proxy?.egress_mode || '';
    const egress = document.createElement('td');
    egress.textContent = mode === 'tailnet' ? 'Tailnet Gateway' : mode === 'system' ? 'Sistema' : 'Heredado';
    egress.title = mode ? 'Ruta saliente seleccionada en este servicio' : 'Se utiliza la plantilla o el valor global';
    tr.append(egress);
    const td = document.createElement('td');
    if(currentRole!=='viewer') td.append(
      button('Editar', () => edit(item)),
      button('Eliminar', () => run(async () => {
        if (!confirm(window.tr("¿Eliminar {p0}?", {p0:(item.name)}))) return;
        await api('services/' + encodeURIComponent(item.name), 'DELETE');
        notice('Servicio eliminado');
        await refresh();
      }), 'secondary')
    );
    tr.append(td);
    rows.append(tr);
  });
}

function edit(item) {
  el('service-editor').hidden = false;
  el('service-name').value = item?.name || '';
  el('service-name').readOnly = !!item;
  el('backend').value = item?.backend || '';
  el('service-description').value = item?.description || '';
  el('enabled').checked = item?.enabled !== false;
  const select = el('template');
  select.replaceChildren();
  for (const value of ['', ...templates]) {
    const option = document.createElement('option');
    option.value = value;
    option.textContent = value || '(predeterminada)';
    select.append(option);
  }
  select.value = item?.template || '';
  editedProxy = GatewayProxyModel.clone(item?.proxy || {});
  renderProxy(editedProxy);
  el('service-editor').scrollIntoView({behavior: 'smooth'});
}

// Controles visuales del proxy. No se vuelca ni se reescribe JSON manualmente.
function renderProxy(original) {
  const box = el('proxy-fields');
  box.replaceChildren();
  const groups = new Map();
  for (const field of proxySchema) {
    let grid = groups.get(field.group);
    if (!grid) {
      const section = document.createElement('fieldset');
      section.className = 'proxy-group';
      const legend = document.createElement('legend');
      legend.textContent = field.group;
      section.append(legend);
      grid = document.createElement('div');
      grid.className = 'grid';
      section.append(grid);
      box.append(section);
      groups.set(field.group, grid);
    }
    const label = document.createElement('label');
    label.className = 'config-field';
    const span = document.createElement('span');
    span.textContent = field.label;
    label.append(span);
    const old = GatewayProxyModel.pathGet(original, field.key);
    let control;
    if (field.kind === 'boolean' || field.kind === 'select' || field.kind === 'choice') {
      control = document.createElement('select');
      const choices = field.kind === 'boolean'
        ? [{value:'true',label:'Sí'}, {value:'false',label:'No'}]
        : (field.options || []);
      for (const choice of [{value:'',label:'Heredar de plantilla'},...choices]) {
        const opt = document.createElement('option');
        opt.value = choice.value;
        opt.textContent = choice.label;
        control.append(opt);
      }
      if (old !== undefined && ![...control.options].some(o => o.value === String(old))) {
        const opt = document.createElement('option');
        opt.value = String(old);
        opt.textContent = window.tr("{p0} (valor existente)", {p0:(old)});
        control.append(opt);
      }
    } else {
      control = document.createElement('input');
      const types = {integer:'number', url:'url'};
      control.type = types[field.kind] || 'text';
      if (field.kind === 'integer') {
        control.min = '0';
        control.step = '1';
      }
      if (field.kind === 'duration') {
        control.placeholder = 'Ejemplo: 30s, 2m, 1h o 0s';
        control.pattern = '(?:[0-9]+(?:\\.[0-9]+)?(?:ns|us|µs|ms|s|m|h))+';
      }
      if (field.kind === 'url') control.placeholder = 'https://ejemplo.local';
      if (field.key === 'tls.server_name') control.placeholder = 'servidor.local';
    }
    control.dataset.proxyKey = field.key;
    control.value = GatewayProxyModel.stringify(old);
    label.append(control);
    if (field.description) {
      const helper = document.createElement('small');
      helper.className = 'field-help';
      helper.textContent = field.description;
      label.append(helper);
    }
    grid.append(label);
  }
  for (const [name, container] of [['request_headers','request-headers'], ['response_headers','response-headers']]) {
    const target = el(container);
    target.replaceChildren();
    const headers = original[name] || {};
    for (const [key, value] of Object.entries(headers)) addHeaderRow(target, key, value);
  }
  // El editor modifica solo claves conocidas y conserva cualquier extensión
  // existente que todavía no tenga un control visual.
  const known = new Set(proxySchema.map(f => f.key.split('.')[0]).concat(['request_headers','response_headers']));
  const additional = Object.keys(original).filter(k => !known.has(k));
  const tlsExtra = original.tls && typeof original.tls === 'object'
    ? Object.keys(original.tls).filter(k => !proxySchema.some(f => f.key === 'tls.' + k)) : [];
  const message = el('proxy-extra-note');
  message.hidden = !additional.length && !tlsExtra.length;
  message.textContent = window.tr("Se conservarán sin modificar estas opciones adicionales: {p0}", {p0:([...additional, ...tlsExtra.map(k => 'tls.' + k)].join(', '))});
}

function addHeaderRow(container, name = '', value = '') {
  const row = document.createElement('div');
  row.className = 'header-row';
  const keyInput = document.createElement('input');
  keyInput.type = 'text';
  keyInput.placeholder = 'X-Custom-Header';
  keyInput.setAttribute('aria-label', 'Nombre de cabecera');
  keyInput.value = name;
  const valueInput = document.createElement('input');
  valueInput.type = 'text';
  valueInput.placeholder = 'Valor';
  valueInput.setAttribute('aria-label', 'Valor de cabecera');
  valueInput.value = value;
  const remove = button('Quitar', () => row.remove(), 'secondary');
  remove.setAttribute('aria-label', 'Quitar cabecera');
  row.append(keyInput, valueInput, remove);
  container.append(row);
}

const reservedHeaders = new Set([
  'host','connection','upgrade','transfer-encoding','content-length','te','trailer',
  'proxy-authorization','proxy-authenticate','keep-alive','x-forwarded-for',
  'x-forwarded-host','x-forwarded-proto','x-gateway-service'
]);
function collectHeaders(container) {
  const result = {};
  const seen = new Set();
  for (const row of container.querySelectorAll('.header-row')) {
    const [nameField,valueField] = row.querySelectorAll('input');
    const name = nameField.value.trim();
    const value = valueField.value;
    if (!name && !value) continue;
    if (!/^[!#$%&'*+.^_`|~0-9A-Za-z-]+$/.test(name) || /[\r\n]/.test(value)) {
      throw Error(window.tr("Nombre o valor de cabecera HTTP no válido: {p0}", {p0:(name)}));
    }
    if (reservedHeaders.has(name.toLowerCase())) {
      throw Error(window.tr("Cabecera reservada que no puede modificarse: {p0}", {p0:(name)}));
    }
    if (seen.has(name.toLowerCase())) throw Error('Cabecera duplicada: ' + name);
    seen.add(name.toLowerCase());
    result[name] = value;
  }
  return result;
}

function readProxy() {
  const values = {};
  el('proxy-fields').querySelectorAll('[data-proxy-key]').forEach(control => {
    values[control.dataset.proxyKey] = control.value;
  });
  return GatewayProxyModel.encode(editedProxy, proxySchema, values, {
    request_headers: collectHeaders(el('request-headers')),
    response_headers: collectHeaders(el('response-headers'))
  });
}

el('add-request-header').addEventListener('click', () => addHeaderRow(el('request-headers')));
el('add-response-header').addEventListener('click', () => addHeaderRow(el('response-headers')));
el('proxy-streaming').addEventListener('click', () => {
  for (const key of ['timeout','response_header_timeout']) {
    const ctrl = el('proxy-fields').querySelector('[data-proxy-key="' + key + '"]');
    if (ctrl) ctrl.value = '0s';
  }
  notice('Perfil streaming aplicado al formulario. Comprueba los timeouts globales del servidor antes de guardar.');
});

function buildConfigControl(field, value) {
  let control;
  if (field.kind === 'select') {
    control = document.createElement('select');
    for (const choice of field.options || []) {
      const opt = document.createElement('option');
      opt.value = choice.value;
      opt.textContent = choice.label;
      control.append(opt);
    }
    // Si una configuración antigua contiene un valor desconocido, mostrarlo
    // expresamente. No añadirlo al catálogo como si estuviera soportado.
    if (![...control.options].some(opt => opt.value === value)) {
      const old = document.createElement('option');
      old.value = value;
      old.textContent = window.tr("{p0} (valor no disponible)", {p0:(value)});
      old.disabled = true;
      control.prepend(old);
    }
  } else {
    control = document.createElement('input');
    const types = {url: 'url', email: 'email', secret: 'password'};
    control.type = types[field.kind] || 'text';
    control.autocomplete = field.kind === 'secret' ? 'new-password' : 'off';
    if (field.kind === 'secret') {
      control.placeholder = 'Dejar vacío para conservar';
    } else if (field.kind === 'duration') {
      control.placeholder = '720h';
      control.spellcheck = false;
    }
  }
  control.dataset.key = field.key;
  control.value = field.kind === 'secret' ? '' : value;
  if (field.required) control.required = true;
  return control;
}

function renderSettings() {
  const box = el('config-fields');
  box.replaceChildren();
  const groups = new Map();
  for (const field of configSchema) {
    if (!Object.prototype.hasOwnProperty.call(settings, field.key)) continue;
    let section = groups.get(field.group);
    if (!section) {
      section = document.createElement('fieldset');
      section.className = 'config-group';
      const legend = document.createElement('legend');
      legend.textContent = field.group;
      section.append(legend);
      const grid = document.createElement('div');
      grid.className = 'grid';
      section.append(grid);
      box.append(section);
      groups.set(field.group, section);
    }
    const label = document.createElement('label');
    label.className = 'config-field';
    label.dataset.configField = field.key;
    const caption = document.createElement('span');
    caption.textContent = field.label;
    label.append(caption);
    const control = buildConfigControl(field, String(settings[field.key] ?? ''));
    control.addEventListener('change', updateConditionalSettings);
    label.append(control);
    if (field.description) {
      const help = document.createElement('small');
      help.className = 'field-help';
      help.textContent = field.description;
      label.append(help);
    }
    section.lastElementChild.append(label);
  }
  updateConditionalSettings();
}

// Las dependencias de visibilidad vienen del esquema Go. El panel no incluye
// una lista fija de proveedores, campos ni secretos por producto.
function updateConditionalSettings() {
  const form = el('config-fields');
  const values = Object.fromEntries([...form.querySelectorAll('[data-key]')]
    .map(control => [control.dataset.key, control.value]));
  for (const field of configSchema) {
    const label = [...form.querySelectorAll('[data-config-field]')]
      .find(item => item.dataset.configField === field.key);
    if (!label) continue;
    const dependencies = Object.entries(field.visible_when || {});
    // OR: Technitium puede alimentar registros A, certificados, o ambos.
    const visible = dependencies.length === 0 || dependencies.some(([key, choices]) =>
      choices.includes(values[key]));
    label.hidden = !visible;
    const control = label.querySelector('[data-key]');
    if (control) control.disabled = !visible;
  }
  const mode = form.querySelector('[data-key="certificates.mode"]');
  const email = form.querySelector('[data-key="acme.email"]');
  if (email) email.required = mode?.value !== 'force-local';
  const warning = el('admin-disable-warning');
  const enabled = form.querySelector('[data-key="admin.enabled"]');
  warning.hidden = enabled?.value !== 'false';
  renderAdminDomainGuide();
}
function renderAdminDomainGuide() {
  const form = el('config-fields');
  const mode = form.querySelector('[data-key="admin.domain_mode"]')?.value || 'off';
  const subdomain = form.querySelector('[data-key="admin.subdomain"]')?.value.trim() || 'admin';
  const preview = el('admin-domain-preview');
  const oidc = el('admin-oidc-preview');
  if (!lastGatewayDomain || mode === 'off') {
    preview.textContent = tr('Acceso remoto por dominio desactivado o identidad de nodo todavía no disponible.');
    oidc.textContent = '';
    return;
  }
  const root = 'https://' + subdomain + '.' + lastGatewayDomain;
  preview.textContent = tr('Panel HTTPS:') + ' ' + root + '/admin/';
  oidc.textContent = tr('Callback para Dex:') + ' ' + root + '/admin/auth/oidc/callback';
}
el('admin-domain-toggle').addEventListener('click', () => {
  const mode = el('config-fields').querySelector('[data-key="admin.domain_mode"]');
  if (!mode) return;
  mode.value = mode.value === 'off' ? 'tailnet' : 'off';
  updateConditionalSettings();
  notice(tr('Opción seleccionada. Guarda los cambios y reinicia el Gateway para aplicarlos.'));
});

// Login por token, usuario/contraseña local, LDAP o redirección OIDC.
async function showApp() {
  await refresh();
  el('token').value = '';
  el('auth-password').value = '';
  el('auth-totp').value = '';
  el('login-error').textContent = '';
  el('login').hidden = true;
  el('app').hidden = false;
  el('connection').textContent = 'Sesión activa';
  notice('Autenticado');
  document.dispatchEvent(new Event('gateway-admin-ready'));
}
async function getAuthMethods() {
  const r = await fetch(window.gatewayURL('/api/v1/auth/methods'), {cache:'no-store'});
  if (!r.ok) return;
  authMethods = await r.json();
  const mode = authMethods.mode || 'token';
  el('auth-credentials').hidden = !['local','ldap'].includes(mode);
  el('login-totp-wrapper').hidden = mode !== 'local' || !authMethods.mfa_enabled;
  el('auth-oidc').hidden = mode !== 'oidc';
  el('auth-bootstrap-box').hidden = !authMethods.bootstrap;
  el('auth-token-box').open = mode === 'token';
  el('logout').hidden=mode==='proxy';
  el('proxy-session-note').hidden=mode!=='proxy';
  el('auth-method-caption').textContent = ({token:'Acceso mediante token',local:'Cuenta local',ldap:'LDAP de YunoHost',oidc:'Inicio de sesión con OpenID Connect',proxy:'Sesión del sistema anfitrión'})[mode] || mode;
}
el('connect').addEventListener('click', () => run(async () => {
  token = el('token').value.trim();csrf='';
  try { await showApp(); }
  catch (e) { token='';el('login-error').textContent=e.message;throw e; }
}));
el('auth-sign-in').addEventListener('click',()=>run(async()=>{
  const r=await fetch(window.gatewayURL('/api/v1/auth/login'),{method:'POST',credentials:'same-origin',headers:{'Content-Type':'application/json'},body:JSON.stringify({username:el('auth-user').value.trim(),password:el('auth-password').value,totp:el('auth-totp').value.trim(),remember:el('remember-session').checked})});
  const data=await r.json();
  if (!r.ok) {el('login-error').textContent=data.error||'Acceso denegado';throw Error(data.error||'Error de autenticación');}
  token='';csrf=data.csrf;await showApp();
}));
el('bootstrap-redeem').addEventListener('click',()=>run(async()=>{
  const r=await fetch(window.gatewayURL('/api/v1/auth/bootstrap'),{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({code:el('bootstrap-code').value.trim()})});
  const data=await r.json();if(!r.ok)throw Error(data.error||'Código inválido');
  el('bootstrap-token').textContent=data.token;
  el('bootstrap-result').hidden=false;
  el('bootstrap-code').value='';
  el('auth-bootstrap-box').open=true;
  authMethods.bootstrap=false;
}));
el('bootstrap-use').addEventListener('click',()=>run(async()=>{
  token=el('bootstrap-token').textContent.trim();csrf='';
  // Evitar que el secreto permanezca visible al cerrar la sesión.
  el('bootstrap-token').textContent='';
  el('bootstrap-result').hidden=true;
  el('auth-bootstrap-box').hidden=true;
  await showApp();
}));
el('logout').addEventListener('click',()=>run(async()=>{
  el('recovery-token-value').textContent='';el('recovery-result').hidden=true;
  try {await api('auth/logout','POST',{});}catch{}
  token='';csrf='';
  el('app').hidden=true;el('login').hidden=false;
  el('connection').textContent='Sin autenticar';
  await getAuthMethods();
}));
async function refreshAuthSettings() {
  authSettings=await api('auth/settings');
  const a=authSettings;
  el('auth-mode').value=a.mode;
  el('auth-local-user').value=a.local_username || '';
  el('auth-local-password').value='';
  el('auth-ldap-url').value=a.ldap?.url||'';
  el('auth-ldap-base').value=a.ldap?.base_dn||'';
  el('auth-ldap-bind').value=a.ldap?.bind_dn||'';
  el('auth-ldap-allowed').value=(a.ldap?.allowed_users||[]).join('\n');
  el('auth-ldap-groups').value=(a.ldap?.allowed_groups||[]).join('\n');
  el('auth-ldap-secret').value='';
  el('auth-oidc-issuer').value=a.oidc?.issuer||'';
  el('auth-oidc-id').value=a.oidc?.client_id||'';
  el('auth-oidc-redirect').value=a.oidc?.redirect_url||'';
  el('auth-oidc-allowed').value=(a.oidc?.allowed_emails||[]).join('\n');
  el('auth-oidc-groups').value=(a.oidc?.allowed_groups||[]).join('\n');
  el('auth-role-groups').value=Object.entries(a.role_groups||{}).map(([g,r])=>g+'='+r).join('\n');
  el('auth-role-emails').value=Object.entries(a.role_emails||{}).map(([e,r])=>e+'='+r).join('\n');
  el('auth-oidc-provider').value=a.oidc?.issuer==='https://accounts.google.com'?'google':'custom';
  el('auth-oidc-disable-groups').checked=!!a.oidc?.disable_groups_scope;
  el('mfa-status').textContent=a.mfa_enabled?'TOTP activo para la cuenta local':'TOTP desactivado';
  el('mfa-setup').hidden=!!a.mfa_enabled;
  el('mfa-disable').hidden=!a.mfa_enabled;
  el('auth-oidc-secret').value='';
  showAuthGroups();
}
function showAuthGroups() {
  document.querySelectorAll('[data-auth-mode]').forEach(x=>x.hidden=x.dataset.authMode!==el('auth-mode').value);
  el('mfa-management-card').hidden=authMethods.mode==='proxy'||el('auth-mode').value!=='local';
}
el('auth-mode').addEventListener('change',showAuthGroups);
el('auth-oidc-provider').addEventListener('change',()=>{
  const provider=el('auth-oidc-provider').value;
  const google=provider==='google';
  if(google){el('auth-oidc-issuer').value='https://accounts.google.com';el('auth-oidc-disable-groups').checked=true;}
  el('auth-oidc-provider-help').textContent=google
    ? 'Google: usa client_id y client_secret de OAuth Web. Sin grupos OIDC: deja Grupos admitidos vacío, indica el correo en Correos autorizados y asigna un rol en Permisos por correos (correo=admin).'
    : provider==='opensource'
      ? 'Authelia / Authentik: activa MFA en el proveedor, utiliza su Issuer HTTPS exacto y registra la URL de callback. Comprueba si envían grupos o utiliza permisos por correo.'
      : 'Dex, Authelia y Authentik pueden gestionar MFA. Configura el issuer HTTPS exacto y grupos o correos autorizados.';
});
el('auth-use-domain-callback').addEventListener('click', () => {
  const mode = el('config-fields').querySelector('[data-key="admin.domain_mode"]')?.value || 'off';
  const subdomain = el('config-fields').querySelector('[data-key="admin.subdomain"]')?.value.trim() || 'admin';
  if (mode === 'off' || !lastGatewayDomain) {
    notice(tr('Activa el acceso por dominio y confirma el nombre del nodo antes de configurar Dex.'), true);
    return;
  }
  el('auth-oidc-redirect').value = 'https://' + subdomain + '.' + lastGatewayDomain + '/admin/auth/oidc/callback';
  notice(tr('URL de callback preparada. Registra esa misma URL en Dex.'));
});
el('auth-settings-reload').addEventListener('click',()=>run(refreshAuthSettings));
const lines = id => el(id).value.split(/[\n,]/).map(v=>v.trim()).filter(Boolean);
el('auth-settings-form').addEventListener('submit',event=>{
  event.preventDefault();
  run(async()=>{
    const update={
      mode:el('auth-mode').value,
      local_username:el('auth-local-user').value.trim(),local_password:el('auth-local-password').value,
      ldap:{url:el('auth-ldap-url').value.trim(),base_dn:el('auth-ldap-base').value.trim(),bind_dn:el('auth-ldap-bind').value.trim(),bind_password:el('auth-ldap-secret').value,allowed_users:lines('auth-ldap-allowed'),allowed_groups:lines('auth-ldap-groups')},
      role_groups:parseRoleGroups(el('auth-role-groups').value),
      role_emails:parseRoleGroups(el('auth-role-emails').value),
      oidc:{issuer:el('auth-oidc-issuer').value.trim(),client_id:el('auth-oidc-id').value.trim(),client_secret:el('auth-oidc-secret').value,redirect_url:el('auth-oidc-redirect').value.trim(),allowed_emails:lines('auth-oidc-allowed'),allowed_groups:lines('auth-oidc-groups'),disable_groups_scope:el('auth-oidc-disable-groups').checked}
    };
    if (!confirm(authMethods.mode==='proxy'
      ? tr('¿Guardar el método para el acceso directo al Gateway?')
      : '¿Guardar este método de acceso? Se cerrarán las sesiones existentes.'))return;
    await api('auth/settings','PUT',update);
    if(authMethods.mode==='proxy'){
      await restoreSession();
      notice(tr('Acceso directo actualizado. Tu sesión del sistema anfitrión sigue activa.'));
      return;
    }
    el('recovery-token-value').textContent='';el('recovery-result').hidden=true;
    token='';csrf='';
    el('app').hidden=true;el('login').hidden=false;
    el('connection').textContent='Sin autenticar';
    await getAuthMethods();
    el('login-error').textContent='Configuración guardada. Inicia sesión con el método seleccionado.';
  });
});
async function restoreSession() {
  await getAuthMethods();
  try {
    const res=await fetch(window.gatewayURL('/api/v1/auth/session'),{credentials:'same-origin',cache:'no-store'});
    if(!res.ok)return;
    const data=await res.json();
    if(data.authenticated){csrf=data.csrf||'';await showApp();}
  }catch{}
}
restoreSession();

document.querySelectorAll('[data-tab]').forEach(b => b.addEventListener('click', () => {
  if (b.dataset.tab !== 'auth') {el('recovery-token-value').textContent='';el('recovery-result').hidden=true;}
  document.querySelectorAll('[data-tab]').forEach(x => x.classList.toggle('selected', x === b));
  document.querySelectorAll('[data-page]').forEach(x => x.hidden = x.dataset.page !== b.dataset.tab);
  notice('');
  if (b.dataset.tab === 'network') run(refreshNetwork);
  if (b.dataset.tab === 'headscale-users') run(refreshHeadscaleAdmin);
  if (b.dataset.tab === 'overview' && currentRole==='admin') run(refreshBackupOverview);
  if (b.dataset.tab === 'templates') run(loadTemplateEditor);
  if (b.dataset.tab === 'exposure') run(async()=>{await loadExposure();await loadAudit();await loadAPITokens();});
}));

el('refresh-acme').addEventListener('click', () => run(async () => {
  const status = await api('certificates/refresh', 'POST', {});
  renderStatus(status);
  if (status.acme_retry_after_utc) {
    notice('ACME limitado: siguiente reintento permitido ' + status.acme_retry_after_utc);
  } else {
    notice('Estado TLS comprobado: ' + (status.certificate_source || 'sin datos'));
  }
}));

el('reload').addEventListener('click', () => run(async () => {
  await api('reload', 'POST', {});
  await refresh();
  notice('Configuración aplicada');
}));

el('new-service').addEventListener('click', () => edit(null));
el('cancel').addEventListener('click', () => {el('service-editor').hidden = true;});
el('service-form').addEventListener('submit', event => {
  event.preventDefault();
  run(async () => {
    const name = el('service-name').value.trim();
    const old = services.some(x => x.name === name);
    const body = {
      name, backend: el('backend').value.trim(),
      template: el('template').value, description: el('service-description').value.trim(), enabled: el('enabled').checked
    };
    body.proxy = readProxy();
    await api('services' + (old ? '/' + encodeURIComponent(name) : ''), old ? 'PUT' : 'POST', body);
    el('service-editor').hidden = true;
    await refresh();
    notice('Servicio guardado y recargado');
  });
});

el('config-form').addEventListener('submit', event => {
  event.preventDefault();
  run(async () => {
    const changes = {};
    el('config-fields').querySelectorAll('[data-key]').forEach(control => {
      const key = control.dataset.key;
      if (control.disabled) return;
      if (control.type === 'password' && control.value === '') return;
      if (control.value !== settings[key]) changes[key] = control.value;
    });
    if (!Object.keys(changes).length) {
      notice('No hay cambios');
      return;
    }
    if (changes['admin.enabled'] === 'false' &&
        !confirm('Desactivar la API y el panel impedirá volver a entrar tras reiniciar. ¿Continuar?')) {
      return;
    }
    await api('config', 'PUT', {changes});
    await refresh();
    if (window.markRestartRequired) window.markRestartRequired();
    notice('Configuración guardada en SQLite. Reinicia el Gateway para aplicar los cambios.');
  });
});


// Red / Headscale: la lista procede de la API REST Headscale; los detalles
// de conexión son observaciones de la instancia tsnet del Gateway.
let networkData = [];
let selectedNode = null;
let nodeFilter = 'all';
const netText = v => v === null || v === undefined || v === '' ? 'No disponible' :
  Array.isArray(v) ? (v.length ? v.join(', ') : '—') : String(v);
const netDate = v => v ? new Date(v).toLocaleString(window.gatewayLanguage?.() || 'es') : 'No disponible';
function dlPairs(id, pairs) {
  const target = el(id);
  target.replaceChildren();
  for (const [label, value] of pairs) {
    const dt = document.createElement('dt');
    const dd = document.createElement('dd');
    dt.textContent = label;
    dd.textContent = netText(value);
    target.append(dt, dd);
  }
}
function metric(label, value) {
  const box = document.createElement('div');
  box.className = 'network-stat';
  const strong = document.createElement('strong'); strong.textContent = String(value);
  const span = document.createElement('span'); span.textContent = label;
  box.append(strong, span);
  return box;
}
async function initializeNetwork() {
  const info = await api('network/info');
  el('network-nav').hidden = info.enabled !== true;
  el('policy-dns-nav').hidden = currentRole !== 'admin' || info.enabled !== true;
  for(const id of ['headscale-users-nav','policy-templates-nav'])el(id).hidden=currentRole!=='admin'||info.enabled!==true;
  el('magicdns-nav').hidden = currentRole !== 'admin' || info.enabled !== true;
  if (!info.enabled) return;
  el('network-info').textContent = 'Headscale: ' + info.headscale_url +
    ' · Gateway tsnet: ' + info.gateway_vpn_state +
    (info.api_key_configured ? ' · API key configurada' : ' · Falta API key');
  const connected=info.gateway_vpn_state==='Running';
  el('network-login').textContent=connected?'Pausar · Gateway tsnet':info.manually_paused||info.gateway_vpn_state==='Stopped'?'Reconectar · Gateway tsnet':'Conectar · Gateway tsnet';
  const pauseBlocked=connected&&info.pause_allowed===false;
  el('network-login').dataset.pauseBlocked=String(pauseBlocked);
  el('network-login').disabled=pauseBlocked;
  el('network-login').title=pauseBlocked?info.pause_block_reason||'Pausa no disponible desde la VPN':'';
  el('network-login').classList.toggle('danger',connected);
  el('network-login').hidden=currentRole!=='admin';
  const prefs = info.gateway_preferences || {};
  dlPairs('network-gateway-prefs', Object.keys(prefs).length ? [
    ['Gateway · Acepta rutas', prefs.accept_routes ? 'Sí' : 'No'],
    ['Gateway · Acepta DNS', prefs.accept_dns ? 'Sí' : 'No'],
    ['Gateway · Rutas anunciadas', prefs.advertise_routes],
    ['Gateway · Acceso LAN con exit node', prefs.exit_node_allow_lan_access ? 'Sí' : 'No']
  ] : []);
  if (Object.keys(prefs).length) {
    /** @type {HTMLInputElement} */(el('gateway-pref-routes')).checked = !!prefs.accept_routes;
    /** @type {HTMLInputElement} */(el('gateway-pref-dns')).checked = !!prefs.accept_dns;
    /** @type {HTMLInputElement} */(el('gateway-pref-exit-lan')).checked = !!prefs.exit_node_allow_lan_access;
    /** @type {HTMLInputElement} */(el('gateway-pref-advertise')).value = (prefs.advertise_routes||[]).join(', ');
  }
  const link = el('network-login-link');
  if (info.login_url) {
    link.href = info.login_url; link.hidden = false;
    link.textContent = 'Abrir autorización de Headscale';
    el('network-login-status').textContent = 'Autoriza el Gateway abriendo el enlace; se actualizará el estado al terminar.';
  } else {
    link.removeAttribute('href'); link.hidden = true;
    el('network-login-status').textContent = info.gateway_vpn_state === 'Running'
      ? 'Gateway autenticado en Headscale.' : info.manually_paused?'VPN pausada. Pulsa Reconectar para reanudar con la misma identidad.':'Si el nodo está pendiente, pulsa Conectar para solicitar la autorización.';
  }
  if(pauseBlocked)el('network-login-status').textContent=info.pause_block_reason;
  el('network-gateway-summary').replaceChildren(metric('IP del Gateway',netText(info.gateway_ips)),metric('Estado tsnet',info.gateway_vpn_state));
  return info;
}
async function refreshNetwork() {
  await initializeNetwork();
  const data = await api('network/nodes');
  networkData = data.nodes || [];
  if (typeof showPendingRoutes === 'function') showPendingRoutes(networkData, true);
  const summary = el('network-summary');
  summary.replaceChildren(metric('Equipos totales',networkData.length),metric('Conectados',data.online ?? 0),metric('Desconectados',data.offline ?? 0));
  renderNetworkRows();
  if (selectedNode) {
    selectedNode = networkData.find(n => n.id === selectedNode.id) || null;
    if (selectedNode) renderNodeDetails(selectedNode);
    else el('network-detail').hidden = true;
  }
}
function renderNetworkRows() {
  const target = el('network-rows');target.replaceChildren();
  const query = el('network-search').value.trim().toLowerCase();
  const filtered = networkData.filter(n => (nodeFilter === 'all' || (n.online ? 'online' : 'offline') === nodeFilter) &&
    [n.name,n.given_name,n.user,...(n.ip_addresses||[]),...(n.tags||[])].join(' ').toLowerCase().includes(query));
  for (const n of filtered) {
    const tr = document.createElement('tr');
    for (const value of [n.given_name||n.name,n.online?'Conectado':'Desconectado',(n.ip_addresses||[]).join(', '), n.peer ? (n.peer.current_endpoint||n.peer.derp||'—'):'Sin datos tsnet',netDate(n.last_seen)]) {
      const td = document.createElement('td');td.textContent = value;tr.append(td);
    }
    tr.children[1].className = 'network-status ' + (n.online?'online':'offline');
    const td = document.createElement('td');td.append(button('Detalles',()=>renderNodeDetails(n)));tr.append(td);target.append(tr);
  }
  el('network-empty').textContent = filtered.length ? '' : 'No hay nodos para este filtro (o Headscale no ha devuelto equipos).';
}
function renderNodeDetails(n) {
  selectedNode = n;
  el('network-detail').hidden = false;
  el('headscale-node-actions').hidden = currentRole !== 'admin';
  el('network-detail-title').textContent = (window.tr?.('Equipo') || 'Equipo') + ' · ' + (n.given_name||n.name);
  const p = n.peer;
  const v4 = (n.ip_addresses||[]).filter(ip => !ip.includes(':'));
  const v6 = (n.ip_addresses||[]).filter(ip => ip.includes(':'));
  dlPairs('network-detail-identity',[
    ['ID Headscale',n.id],['Nombre',n.name],['Nombre visible',n.given_name],['Usuario',n.user],
    ['Conectado',n.online?'Sí':'No'],['IPv4',v4],['IPv6',v6],['SO observado',p?.os],
    ['Alta en Headscale',netDate(n.created_at)],['Última conexión',netDate(n.last_seen)],
    ['Expiración',netDate(n.expiry)],['Método de alta',n.register_method],['Etiquetas',n.tags]
  ]);
  dlPairs('network-detail-transport',[
    ['Datos de tsnet',p?'Disponibles':'Sin observación local'],['DERP',p?.derp],['Peer relay',p?.peer_relay],
    ['Endpoint UDP actual',p?.current_endpoint],['Endpoints anunciados',p?.endpoints],
    ['Último handshake',netDate(p?.last_handshake)],['Último tráfico',netDate(p?.last_write)],
    ['Descargado por el Gateway',p ? (p.rx_bytes+' bytes') : undefined],['Enviado por el Gateway',p ? (p.tx_bytes+' bytes') : undefined]
  ]);
  dlPairs('network-detail-routes',[
    ['Rutas anunciadas / disponibles',n.available_routes],['Rutas aprobadas en Headscale',n.approved_routes],
    ['Subredes',n.subnet_routes],['Ofrece nodo de salida', (n.available_routes||[]).includes('0.0.0.0/0')||(n.available_routes||[]).includes('::/0')?'Sí':'No'],
    ['Nodo de salida disponible (tsnet)',p?.exit_node_option ? 'Sí' : (p?'No':'No disponible')],
    ['Aceptar DNS/rutas', 'Preferencias locales del equipo remoto: no disponibles por API Headscale']
  ]);
  el('network-commands').textContent =
    'En este equipo remoto, con permisos de administrador:\n' +
    'tailscale set --advertise-exit-node\n' +
    'tailscale set --advertise-routes=<TU_SUBRED_CIDR>\n' +
    'tailscale set --accept-routes\n' +
    'tailscale set --accept-dns=true\n' +
    'tailscale set --exit-node-allow-lan-access=true  # solo si usas exit node\n\n' +
    'Para anunciar LAN o salida también se requiere IP forwarding; Headscale debe aprobar las rutas.\n' +
    'No ejecutes las opciones de rutas sin ajustar antes las subredes del equipo.';
  el('network-diagnostic-summary').textContent='Elige una prueba para ver qué significa el resultado y qué puedes revisar.';
  el('network-ping-result').textContent = 'Selecciona DISCO, TSMP o ICMP para comprobar este equipo desde el tsnet del Gateway.';
  el('network-detail').scrollIntoView({behavior:'smooth',block:'start'});
}
el('network-refresh').addEventListener('click',()=>run(refreshNetwork));
el('network-search').addEventListener('input',renderNetworkRows);
document.querySelectorAll('[data-netfilter]').forEach(b=>b.addEventListener('click',()=>{
  nodeFilter=b.dataset.netfilter;
  document.querySelectorAll('[data-netfilter]').forEach(btn=>{
    btn.className = btn===b?'selected':'secondary';
  });
  renderNetworkRows();
}));
el('network-close-detail').addEventListener('click',()=>{selectedNode=null;el('network-detail').hidden=true;});
async function runNodeDiagnostics(modes){
  if(!selectedNode)return;
  const nodeID=selectedNode.id, nodeName=selectedNode.given_name||selectedNode.name;
  const output=el('network-ping-result');output.textContent='Comprobando '+nodeName+'…\n';
  el('network-ping').disabled=true;el('network-test-suite').disabled=true;
  const reports=[];el('network-diagnostic-summary').replaceChildren();
  try{for(const mode of modes){
    let data;try{data=await api('network/ping','POST',{node_id:nodeID,mode})}catch(error){data={mode,error:error.message}}
    renderDiagnosticExplanation(mode,data);
    reports.push((data.error?'✕ ':'✓ ')+mode+' · '+(data.error||'Correcto')+'\n'+JSON.stringify(data,null,2));
    output.textContent=nodeName+' · '+reports.length+'/'+modes.length+' pruebas\n\n'+reports.join('\n\n');
  }}finally{el('network-ping').disabled=false;el('network-test-suite').disabled=false;}
}
el('network-ping').addEventListener('click',()=>run(()=>runNodeDiagnostics([el('network-ping-type').value])));
el('network-test-suite').addEventListener('click',()=>run(()=>runNodeDiagnostics(['disco','tsmp','icmp','tcp-22','tcp-443','tls-443'])));
el('network-login').addEventListener('click',()=>run(async()=>{
  const button=el('network-login');button.disabled=true;
  try{
    const info=await api('network/info');
    if(info.gateway_vpn_state==='Running'){
      if(info.pause_allowed===false){notice(info.pause_block_reason,true);await initializeNetwork();return;}
      if(await panelDialog({title:'Pausar la VPN del Gateway',message:'Se interrumpirán las conexiones que pasan por su VPN. El panel local y la identidad se conservan. Para volver a conectar remotamente necesitarás acceso LAN o local. La pausa se mantiene al reiniciar.',input:false,accept:'Pausar VPN'})===null)return;
      const data=await api('network/disconnect','POST',{});notice(data.message);await initializeNetwork();return;
    }
    const data=await api('network/login','POST',{});notice(data.message||data.state);
    if(data.login_url){const link=el('network-login-link');link.href=data.login_url;link.hidden=false;}
    for(let i=0;i<8;i++){
      const next=await initializeNetwork();
      if(next?.gateway_vpn_state==='Running'||next?.login_url)break;
      await new Promise(r=>setTimeout(r,1000));
    }
  }finally{button.disabled=button.dataset.pauseBlocked==='true';}
}));
el('network-settings').addEventListener('click',()=>{
  const tab=document.querySelector('[data-tab="settings"]'); tab.click();
  const ctrl=document.querySelector('[data-key="headscale.url"]'); ctrl?.scrollIntoView({behavior:'smooth'});
});

// Formulario Headscale separado de las opciones generales; ambos editan
// config.yaml mediante la misma API validada del Gateway.
el('network-connection-form').addEventListener('submit', event => {
  event.preventDefault();
  run(async () => {
    const changes = {};
    const url = el('network-url').value.trim();
    const enabled = String(el('network-enabled').checked);
    if (url !== settings['headscale.url']) changes['headscale.url'] = url;
    if (enabled !== settings['headscale.panel_enabled']) changes['headscale.panel_enabled'] = enabled;
    if (el('network-key').value) changes['headscale.api_key'] = el('network-key').value;
    if (!Object.keys(changes).length) {notice('No hay cambios en la conexión');return;}
    if (enabled === 'false' && !confirm('La pestaña desaparecerá tras reiniciar el Gateway. ¿Desactivar?')) return;
    await api('config','PUT',{changes});
    await refresh();
    if (window.markRestartRequired) window.markRestartRequired();
    notice('Conexión Headscale guardada. Reinicia Headscale Gateway para aplicar los cambios.');
  });
});

// Reinicio explícito del proceso: la API responde 202 y reejecuta el mismo binario.
el('setup-configure').addEventListener('click',()=>document.querySelector('[data-tab="settings"]').click());
async function requestGatewayRestart() { return run(async()=>{
  if (!confirm('¿Reiniciar Headscale Gateway? Se interrumpirán temporalmente el proxy y las conexiones VPN del Gateway.')) return;
  const data=await api('system/restart','POST',{});
  if (window.notifyRestartRequested) window.notifyRestartRequested();
  notice(data.restarting ? 'Reinicio solicitado. El panel puede desconectarse durante unos segundos.' : 'Solicitud enviada');
}); }
el('restart-gateway').addEventListener('click',requestGatewayRestart);
el('restart-settings').addEventListener('click',requestGatewayRestart);

// Native HTML dialogs work in embedded browsers without window.prompt().
function panelDialog({title, message='', value='', expected=null, choices=null, accept='Continuar', input=true}) {
  return new Promise(resolve=>{
    const dialog=document.createElement('dialog');dialog.className='panel-dialog';
    const form=document.createElement('form');form.method='dialog';
    const heading=document.createElement('h2');heading.textContent=title;heading.id='panel-dialog-title';
    dialog.setAttribute('aria-labelledby',heading.id);
    const description=document.createElement('p');description.textContent=message;description.className='dialog-description';
    form.append(heading,description);
    let field;const checks=[];
    if(choices!==null){
      for(const choice of choices){const label=document.createElement('label');label.className='check';const box=document.createElement('input');box.type='checkbox';box.value=choice.value;box.checked=choice.checked;checks.push(box);label.append(box,document.createTextNode(choice.label));form.append(label);}
    }else if(input){
      const label=document.createElement('label');label.textContent=expected===null?'Valor':'Escribe la huella SHA-256 completa para confirmar';
      field=document.createElement('input');field.value=value;field.autocomplete='off';field.spellcheck=false;label.append(field);form.append(label);
    }
    const actions=document.createElement('div');actions.className='actions';
    const cancel=document.createElement('button');cancel.type='button';cancel.textContent='Cancelar';cancel.className='secondary';
    const save=document.createElement('button');save.type='submit';save.textContent=accept;
    const validate=()=>{save.disabled=expected!==null&&field.value.trim()!==expected;};
    if(field)field.addEventListener('input',validate);validate();
    actions.append(cancel,save);form.append(actions);dialog.append(form);document.body.append(dialog);
    let result=null,settled=false;
    const finish=()=>{if(settled)return;settled=true;if(dialog.open)dialog.close();dialog.remove();resolve(result);};
    cancel.addEventListener('click',finish);
    dialog.addEventListener('cancel',event=>{event.preventDefault();finish();});
    form.addEventListener('submit',event=>{event.preventDefault();if(save.disabled)return;result=choices!==null?checks.filter(c=>c.checked).map(c=>c.value):field?field.value:true;finish();});
    dialog.addEventListener('close',finish,{once:true});
    dialog.showModal();if(field)field.focus();else cancel.focus();
  });
}

function diagnosticExplanation(mode,data){
 const error=String(data.error||'');
 const port=mode.split('-')[1]||'';
 if(!error){
  if(mode==='dns-reverse')return {title:'Consulta DNS completada',detail:(data.names||[]).length?window.tr("El resolver del Gateway devuelve: {p0}", {p0:(data.names.join(', '))}):'El resolver no devolvió nombres para esta IP.',action:'La ausencia de nombre inverso no significa que el equipo esté apagado.'};
  if(mode.startsWith('tls-'))return {title:'Conexión cifrada verificada',detail:'El servicio responde y su certificado es válido para la IP consultada según la confianza del Gateway.',action:'Esto comprueba TLS; no comprueba el inicio de sesión ni todas las funciones del servicio.'};
  if(mode.startsWith('tcp-'))return {title:'Puerto '+port+' accesible',detail:'El Gateway ha podido abrir una conexión TCP con este equipo.',action:'Si la aplicación falla, revisa sus credenciales y configuración. Un puerto abierto no garantiza que la aplicación funcione.'};
  return {title:'El equipo responde',detail:'Respuesta a '+mode.toUpperCase()+(data.latency_ms!=null?' en '+Number(data.latency_ms).toFixed(1)+' ms':'')+(data.derp_region?' mediante un servidor de retransmisión DERP.':'.'),action:'Esta prueba confirma la respuesta de red; no comprueba todos los servicios ni todas las reglas ACL.'};
 }
 if(mode==='dns-reverse')return {title:'No se ha obtenido el nombre inverso',detail:'Puede no existir un registro PTR o no responder el resolver.',action:'Comprueba el DNS configurado en el Gateway. Puedes seguir usando la IP; esto no demuestra que el nodo esté desconectado.'};
 if(mode.startsWith('tls-')&&data.connected)return {title:'El puerto responde, pero TLS no se valida',detail:'La conexión TCP funciona; falla la comprobación del certificado o la negociación TLS.',action:'Comprueba la fecha del equipo, la caducidad, que el certificado incluya esta IP (SAN) y que su CA sea de confianza para el Gateway. El servicio puede exigir un nombre DNS. No desactives la validación.'};
 if(/401|403|permiso|autoriz/i.test(error))return {title:'Faltan permisos para la prueba',detail:'La API no ha autorizado la operación.',action:'Revisa la sesión, el rol y los permisos de red. No es una prueba de que el equipo esté caído.'};
 if(/refused|rechaz/i.test(error))return {title:window.tr("Conexión rechazada{p0}", {p0:((port?' en el puerto '+port:''))}),detail:'El destino o un filtro ha rechazado la conexión.',action:'Comprueba que el servicio esté arrancado y escuche en la IP y puerto correctos. Si no ofrece ese servicio, el resultado es normal.'};
 return {title:'No se ha completado la prueba',detail:'Puede deberse a conectividad, un filtro, una regla ACL o al servicio de destino.',action:'Comprueba que ambos equipos estén conectados, que exista una ruta y que el firewall y las ACL permitan esta prueba. ICMP puede estar bloqueado aunque otras conexiones funcionen. Consulta los detalles técnicos para precisar la causa.'};
}
function renderDiagnosticExplanation(mode,data){
 const info=diagnosticExplanation(mode,data),card=document.createElement('article');card.className='diagnostic-explanation';
 const title=document.createElement('h4');title.textContent=mode.toUpperCase()+' · '+info.title;
 const detail=document.createElement('p');detail.textContent=info.detail;const action=document.createElement('p');action.textContent=window.tr("Qué revisar: {p0}", {p0:(info.action)});
 card.append(title,detail,action);el('network-diagnostic-summary').append(card);
}
document.querySelectorAll('[data-diagnostic-tab]').forEach(button=>button.addEventListener('click',()=>{
 const summary=button.dataset.diagnosticTab==='summary';el('network-diagnostic-summary').hidden=!summary;el('network-ping-result').hidden=summary;
 document.querySelectorAll('[data-diagnostic-tab]').forEach(b=>{const active=b===button;b.classList.toggle('selected',active);b.setAttribute('aria-selected',String(active));});
}));
