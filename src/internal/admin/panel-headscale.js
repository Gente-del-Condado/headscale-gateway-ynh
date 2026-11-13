'use strict';
// La clave Headscale queda en el servidor. Todo cambio pasa por la lista de
// operaciones permitidas y exige rol admin, comprobado también en Go.
const hsTr = s => (typeof window.tr === 'function' ? window.tr(s) : s);
const hsEl = id => document.getElementById(id);
const hsTags = text => String(text || '').split(',').map(s => s.trim()).filter(Boolean);
const hsBtn = (caption, callback, secondary = false) => {
  const b = document.createElement('button'); b.type = 'button';
  b.textContent = hsTr(caption); if (secondary) b.className = 'secondary';
  b.addEventListener('click', () => run(callback)); return b;
};
const hsField = (form, name) => /** @type {HTMLInputElement} */ (form.elements.namedItem(name));
const hsValue = value => value == null ? '—' : String(value);
async function headscaleAction(command, message) {
  if (currentRole !== 'admin') throw Error(hsTr('Operación reservada a administradores'));
  const result = await api('network/admin/action', 'POST', command);
  if (message) notice(hsTr(message));
  return result;
}
const hsRow = (target, columns, buttons) => {
  const tr = document.createElement('tr');
  for (const value of columns) {
    const td = document.createElement('td'); td.textContent = hsValue(value); tr.append(td);
  }
  const actions = document.createElement('td'); actions.append(...buttons);
  tr.append(actions); target.append(tr);
};
async function refreshHeadscaleAdmin() {
  const section = hsEl('headscale-management');
  section.hidden = currentRole !== 'admin';
  if (section.hidden) return;
  const messages = [];
  const [users, keys] = await Promise.allSettled([
    api('network/admin/users'), api('network/admin/keys')
  ]);
  const userRows = hsEl('headscale-user-rows'); userRows.replaceChildren();
  if (users.status === 'fulfilled') {
    const records = users.value.users || users.value.user || [];
    for (const user of records) {
      const id = String(user.id ?? '');
      const name = user.name || user.displayName || user.email || '';
      hsRow(userRows, [name, id], [
        hsBtn('Renombrar', async () => {
          const next = await panelDialog({title:hsTr('Nuevo nombre del usuario'),value:name});
          if (!next || next === name) return;
          await headscaleAction({action:'user.rename', id, name:next}, 'Usuario renombrado');
          await refreshHeadscaleAdmin();
        }),
        hsBtn('Eliminar', async () => {
          if (!confirm(hsTr('¿Eliminar este usuario? Esta operación puede afectar a sus nodos.') + '\n' + name)) return;
          await headscaleAction({action:'user.delete', id}, 'Usuario eliminado');
          await refreshHeadscaleAdmin();
        }, true)
      ]);
    }
  } else messages.push(hsTr('No se pudo obtener la lista de usuarios') + ': ' + users.reason.message);
  const keyRows = hsEl('headscale-key-rows'); keyRows.replaceChildren();
  if (keys.status === 'fulfilled') {
    for (const key of (keys.value.preAuthKeys || [])) {
      const id = String(key.id ?? '');
      const owner = key.user?.name || key.user?.id || '—';
      const date = key.expiration ? new Date(key.expiration).toLocaleString(window.gatewayLanguage?.() || 'es') : '—';
      hsRow(keyRows, [id, owner, date], [
        hsBtn('Caducar', async () => {
          if (!confirm(hsTr('¿Caducar esta clave de registro?') + ' #' + id)) return;
          await headscaleAction({action:'key.expire', id}, 'Clave caducada');
          await refreshHeadscaleAdmin();
        }),
        hsBtn('Eliminar', async () => {
          if (!confirm(hsTr('¿Eliminar esta clave de registro?') + ' #' + id)) return;
          await headscaleAction({action:'key.delete', id}, 'Clave eliminada');
          await refreshHeadscaleAdmin();
        }, true)
      ]);
    }
  } else messages.push(hsTr('No se pudo obtener la lista de claves') + ': ' + keys.reason.message);
  hsEl('headscale-admin-status').textContent = messages.length ? messages.join(' · ') : hsTr('Administración sincronizada con Headscale');
}
function hsNodeCommand(action, confirmed, collect) {
  hsEl('headscale-node-' + action).addEventListener('click', () => run(async () => {
    if (!selectedNode || currentRole !== 'admin') return;
    const id = String(selectedNode.id), label = selectedNode.given_name || selectedNode.name;
    const args = collect ? await collect(selectedNode) : {};
    if (args === null) return;
    if (confirmed && !confirm(hsTr(confirmed) + '\n' + label + ' (#' + id + ')')) return;
    await headscaleAction({action:'node.' + action, id, ...args}, 'Cambio aplicado en Headscale');
    selectedNode = null; hsEl('network-detail').hidden = true;
    await refreshNetwork();
  }));
}
hsNodeCommand('rename', '', async node => {
  const name = await panelDialog({title:hsTr('Nuevo nombre para el nodo'),value:node.given_name || node.name});
  return name?.trim() ? {name: name.trim()} : null;
});
hsNodeCommand('tags', '¿Aplicar etiquetas? En versiones recientes de Headscale, etiquetar un nodo puede cambiar su identidad y propiedad.', async node => {
  const tags = await panelDialog({title:hsTr('Etiquetas del nodo (separadas por coma)'),value:(node.tags || []).join(', ')});
  return tags === null ? null : {tags: hsTags(tags)};
});
hsEl('headscale-node-routes').addEventListener('click',()=>run(async()=>{
  if(!selectedNode||currentRole!=='admin')return;
  const id=String(selectedNode.id);
  const inventory=await api('network/nodes');
  const node=(inventory.nodes||[]).find(n=>String(n.id)===id);
  if(!node)throw Error('El nodo ya no está disponible. Actualiza el inventario.');
  const approved=node.approved_routes||[],available=node.available_routes||[];
  const routes=[...new Set([...available,...approved])];
  if(!routes.length){notice('Este equipo no anuncia rutas ni tiene rutas aprobadas.');return;}
  const chosen=await panelDialog({title:window.tr("Aprobar o revocar rutas · {p0}", {p0:((node.given_name||node.name))}),message:'Marca las rutas que deben permanecer aprobadas. Desmarca una ruta para revocarla; desmarcar todas revoca todas. La revocación puede cortar conexiones. Si el nodo sigue anunciándola, volverá a aparecer como pendiente.',choices:routes.map(route=>({value:route,label:route+(approved.includes(route)?' · aprobada':' · pendiente'),checked:approved.includes(route)})),accept:'Guardar aprobaciones'});
  if(chosen===null)return;
  const latestInventory=await api('network/nodes');
  const latest=(latestInventory.nodes||[]).find(n=>String(n.id)===id);
  const same=(a,b)=>JSON.stringify([...new Set(a||[])].sort())===JSON.stringify([...new Set(b||[])].sort());
  if(!latest||!same(latest.approved_routes,approved)||!same(latest.available_routes,available))throw Error('Las rutas han cambiado mientras editabas. Vuelve a abrir el editor.');
  if(same(chosen,approved)){notice('No hay cambios en las rutas.');return;}
  await headscaleAction({action:'node.routes',id,routes:chosen},'Aprobaciones actualizadas; las rutas desmarcadas se han revocado');
  await refreshNetwork();
}));
hsNodeCommand('expire', '¿Caducar la sesión de este nodo? Puede quedar desconectado.');
hsNodeCommand('delete', '¿Eliminar definitivamente este nodo de Headscale?');
hsEl('headscale-admin-refresh').addEventListener('click', () => run(refreshHeadscaleAdmin));
hsEl('headscale-user-create').addEventListener('submit', event => {
  event.preventDefault();
  run(async () => {
    const form = /** @type {HTMLFormElement} */ (event.currentTarget);
    const name = hsField(form, 'name').value.trim();
    await headscaleAction({action:'user.create', name}, 'Usuario creado');
    form.reset(); await refreshHeadscaleAdmin();
  });
});
hsEl('headscale-key-create').addEventListener('submit', event => {
  event.preventDefault();
  run(async () => {
    const form = /** @type {HTMLFormElement} */ (event.currentTarget);
    hsEl('headscale-new-key-wrap').hidden = true;
    hsEl('headscale-new-key').textContent = '';
    const command = {
      action:'key.create', user:hsField(form, 'user').value.trim(),
      hours:Number(hsField(form, 'hours').value),
      tags:hsTags(hsField(form, 'tags').value),
      reusable:hsField(form, 'reusable').checked,
      ephemeral:hsField(form, 'ephemeral').checked
    };
    const result = await headscaleAction(command, 'Clave de registro creada');
    const key = result.preAuthKey?.key || result.preauthkey?.key || result.key || '';
    if (key) {
      hsEl('headscale-new-key').textContent = key;
      hsEl('headscale-new-key-wrap').hidden = false;
    } else notice(hsTr('Headscale no devolvió una clave de registro visible'), true);
    await refreshHeadscaleAdmin();
  });
});
hsEl('headscale-hide-key').addEventListener('click', () => {
  hsEl('headscale-new-key-wrap').hidden = true;
  hsEl('headscale-new-key').textContent = '';
});

// Las peticiones de rutas requieren aprobación en Headscale; nunca se aplican silenciosamente.
const pendingRoutesSeen = new Set();
function showPendingRoutes(nodes, showToast = false) {
  const panel = hsEl('network-route-alert'), list = hsEl('network-route-requests');
  if (!panel || !list) return;
  list.replaceChildren();
  const requests=[];
  if (currentRole === 'admin') for (const node of nodes || []) {
    const approved = new Set(node.approved_routes || []);
    for (const route of node.available_routes || []) if (!approved.has(route)) requests.push({node,route});
  }
  updateRouteNotificationCenter(requests, showToast);
  panel.hidden = requests.length === 0;
  hsEl('network-route-count').textContent = String(requests.length);
  const currentKeys=new Set(requests.map(({node,route})=>`${node.id}:${route}`));
  for(const key of pendingRoutesSeen)if(!currentKeys.has(key))pendingRoutesSeen.delete(key);
  if (showToast) {
    const newly = requests.filter(({node,route}) => !pendingRoutesSeen.has(`${node.id}:${route}`));
    if (newly.length) notice(hsTr('Rutas Headscale pendientes de aprobación')+`: ${newly.length}`);
  }
  for (const {node,route} of requests) {
    pendingRoutesSeen.add(`${node.id}:${route}`);
    const item = document.createElement('div');item.className='route-request';
    const description=document.createElement('span');description.textContent=`${node.given_name||node.name} (#${node.id}) · ${route}`;
    const button=hsBtn('Aprobar ruta',async()=>{
      if (!confirm(hsTr('¿Autorizar que este nodo anuncie la ruta?')+`\n${node.name}\n${route}`))return;
      const inventory = await api('network/nodes');
      const latest = (inventory.nodes || []).find(n => String(n.id) === String(node.id));
      if (!latest || !(latest.available_routes || []).includes(route)) throw Error('La solicitud ya no está disponible. Actualiza el inventario.');
      const approved = [...new Set([...(latest.approved_routes||[]),route])];
      await headscaleAction({action:'node.routes',id:String(node.id),routes:approved},'Ruta autorizada');
      pendingRoutesSeen.delete(`${node.id}:${route}`);
      await refreshNetwork();
    },true);
    item.append(description,button);list.append(item);
  }
}
const prefInput=id=>/** @type {HTMLInputElement} */(hsEl(id));
hsEl('gateway-prefs-form').addEventListener('submit',event=>{
  event.preventDefault();run(async()=>{
    const advertise=prefInput('gateway-pref-advertise').value.split(',').map(v=>v.trim()).filter(Boolean);
    const data={accept_routes:prefInput('gateway-pref-routes').checked,accept_dns:prefInput('gateway-pref-dns').checked,
      exit_node_allow_lan_access:prefInput('gateway-pref-exit-lan').checked,advertise_routes:advertise};
    if(!confirm(hsTr('¿Aplicar estos ajustes al Gateway tsnet? Las rutas anunciadas necesitarán aprobación en Headscale.')))return;
    await api('network/gateway/preferences','PUT',data);notice(hsTr('Preferencias del Gateway actualizadas'));await initializeNetwork();
  });
});

// Consulta solo con una sesión autenticada; el navegador puede frenar las pestañas ocultas.
let routePollBusy = false;
async function pollRouteNotifications() {
  const center=hsEl('route-notifications');
  if (hsEl('app').hidden || currentRole !== 'admin') {center.hidden=true;pendingRoutesSeen.clear();return;}
  if(routePollBusy)return;
  routePollBusy=true;
  try {const data=await api('network/nodes');if(!hsEl('app').hidden && currentRole==='admin')showPendingRoutes(data.nodes||[],true);}
  catch(e){hsEl('route-poll-status').textContent='No se han podido actualizar las solicitudes. Los datos pueden estar desactualizados.';if(e.status===401||e.status===403){center.hidden=true;pendingRoutesSeen.clear();}}
  finally{routePollBusy=false;}
}
function updateRouteNotificationCenter(requests, notify) {
  const center=hsEl('route-notifications');center.hidden=currentRole!=='admin';
  hsEl('route-notification-count').textContent=String(requests.length);
  hsEl('route-poll-status').textContent=window.tr("Última comprobación: {p0}", {p0:(new Date().toLocaleTimeString())});
  const list=hsEl('route-notification-list');list.replaceChildren();
  for(const {node,route} of requests){const item=document.createElement('li');item.textContent=`${node.given_name||node.name} · ${route}`;list.append(item);}
  const count=requests.filter(({node,route})=>!pendingRoutesSeen.has(`${node.id}:${route}`)).length;
  if(notify&&count&&localStorage.getItem('hgw-route-notifications')==='on'&&'Notification' in window&&Notification.permission==='granted') {
    const n=new Notification('Headscale Gateway',{body:window.tr("{p0} nuevas rutas necesitan aprobación.", {p0:(count)}),tag:'hgw-routes-'+location.host});
    n.onclick=()=>{window.focus();hsEl('network-nav').click();n.close();};
  }
}
hsEl('route-notifications-enable').addEventListener('click',()=>run(async()=>{
  if(!('Notification' in window))throw Error('Este navegador no admite notificaciones. Usa la aplicación de bandeja.');
  const permission=await Notification.requestPermission();
  if(permission!=='granted')throw Error('Permiso de notificaciones no concedido. Revisa los permisos del navegador.');
  localStorage.setItem('hgw-route-notifications','on');notice('Notificaciones activadas en este navegador');
}));
hsEl('route-notifications-disable').addEventListener('click',()=>{localStorage.removeItem('hgw-route-notifications');notice('Notificaciones silenciadas');});
hsEl('route-notifications-review').addEventListener('click',()=>{hsEl('network-nav').click();hsEl('network-route-alert').scrollIntoView({block:'center'});});
setInterval(pollRouteNotifications,60000);
document.addEventListener('visibilitychange',()=>{if(!document.hidden)pollRouteNotifications();});
