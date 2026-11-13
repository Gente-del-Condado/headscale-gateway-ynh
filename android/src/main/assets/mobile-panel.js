/* Navegación sobre los controles reales del panel, sin duplicar permisos ni API. */
(() => {
  document.body.classList.add('android-container');
  const spanish=()=>{
    if(window.gatewayLanguage && window.gatewayLanguage()!=='es') {
      const selector=document.getElementById('ui-language');selector.value='es';selector.dispatchEvent(new Event('change'));
    }
  };
  if(!window.gatewayAndroidLanguageObserver) {
    window.gatewayAndroidLanguageObserver=new MutationObserver(spanish);
    window.gatewayAndroidLanguageObserver.observe(document.documentElement,{attributes:true,attributeFilter:['lang']});
  }
  spanish();
  window.gatewayAndroidSections = () => [...document.querySelectorAll('.main-navigation [data-tab]')]
    .filter(button => !button.closest('[hidden]') && !button.disabled)
    .map(button => ({id:button.dataset.tab, title:button.textContent.trim(), selected:button.classList.contains('selected')}));
  const apply = () => {
    const name = window.gatewayAndroidRequestedSection;
    if (!name) return;
    const button = [...document.querySelectorAll('.main-navigation [data-tab]')]
      .find(button => button.dataset.tab === name && !button.closest('[hidden]') && !button.disabled);
    if (button) {
      window.gatewayAndroidRequestedSection = null;
      button.click(); window.scrollTo(0,0);
    }
  };
  window.gatewayAndroidOpenSection = name => { window.gatewayAndroidRequestedSection=name; apply(); };
  if (!window.gatewayAndroidObserver) {
    window.gatewayAndroidObserver = new MutationObserver(apply);
    window.gatewayAndroidObserver.observe(document.getElementById('app'), {subtree:true,attributes:true,attributeFilter:['hidden']});
  }
})();

// Captura también a.click() sobre enlaces separados del DOM. Se inicia fetch
// antes de que el panel revoque el blob, conservando su nombre y su contenido.
(() => {
  if(window.gatewayAndroidDownload)return;
  const blobs=new Map();
  const create=URL.createObjectURL.bind(URL),revoke=URL.revokeObjectURL.bind(URL);
  URL.createObjectURL=blob=>{const url=create(blob);blobs.set(url,blob);return url;};
  URL.revokeObjectURL=url=>{blobs.delete(url);revoke(url);};
  window.gatewayAndroidDownload=async(url,name)=> {
    let id='';
    try {
      const allowed=url.startsWith('blob:'+location.origin+'/') ||
        url.startsWith(location.origin+'/api/v1/') || url.startsWith(location.origin+'/pki/v1/');
      if(!allowed)throw Error('La descarga no pertenece al Gateway');
      // El objeto ya está en el panel: no se hace fetch(blob:), bloqueado
      // por connect-src 'self', ni se depende de la vida de su URL temporal.
      const blob=blobs.get(url);blobs.delete(url);
      const response=blob?new Response(blob):await fetch(url,{credentials:'same-origin'});
      if(!response.ok)throw Error('No se ha podido descargar el archivo');
      id=DescargasGateway.beginBlob(location.origin,name||'exportacion-gateway.hgw',response.headers.get('Content-Type')||'application/octet-stream');
      if(!id)return;
      const reader=response.body.getReader();
      for(;;) {
        const {value,done}=await reader.read();if(done)break;
        for(let i=0;i<value.length;i+=32768) {
          let binary='';for(const byte of value.subarray(i,i+32768))binary+=String.fromCharCode(byte);
          DescargasGateway.append(id,btoa(binary));
        }
      }
      DescargasGateway.finish(id);
    }catch(error) {
      if(id)DescargasGateway.failed(id);
      else if(typeof notice==='function')notice('No se ha podido descargar el archivo: '+error.message,true);
    }
  };
  const intercept=link=> {
    if(!link || !link.hasAttribute('download'))return false;
    const url=link.href;
    if(!url.startsWith('blob:'+location.origin+'/') && !url.startsWith(location.origin+'/api/v1/') && !url.startsWith(location.origin+'/pki/v1/'))return false;
    void window.gatewayAndroidDownload(url,link.download);return true;
  };
  const click=HTMLAnchorElement.prototype.click;
  HTMLAnchorElement.prototype.click=function(...args){if(!intercept(this))return click.apply(this,args);};
  document.addEventListener('click',event=>{if(intercept(event.target.closest?.('a[download]')))event.preventDefault();},true);
  window.addEventListener('hgw-android-certificate-settings',()=>DescargasGateway.securitySettings(location.origin));
})();
