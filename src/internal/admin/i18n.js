'use strict';
// Catálogos gettext del navegador: msgid está en español y las traducciones inglesas
// proceden de locale/*.po, compatibles con Poedit y compilados por tools/extract_i18n.py.
(() => {
 let supported=['es','en'];
 let language='es', messages={};
 const originalText=new WeakMap(), originalAttributes=new WeakMap(), renderedMessages=new Map();
 try {const saved=localStorage.getItem('headscale-gateway-language');if(saved)language=saved;}catch{}
 window.gatewayLanguage=()=>language;
 window.tr=(msg, params={})=>{
  const output=(messages[String(msg)]||String(msg)).replace(/\{([a-zA-Z0-9_]+)\}/g,(match,key)=>Object.hasOwn(params,key)?String(params[key]):match);
  if(Object.keys(params).length){if(renderedMessages.size>2000)renderedMessages.delete(renderedMessages.keys().next().value);renderedMessages.set(output,{msg,params});}
  return output;
 };
 function translateText(node) {
  if(!node.nodeValue||!node.nodeValue.trim())return;
  if(node.parentElement?.closest('pre, code, script, style, textarea'))return;
  if(!originalText.has(node))originalText.set(node,{source:node.nodeValue,rendered:node.nodeValue});
  // Otros scripts sustituyen textContent tras una respuesta de API. Detecta ese
  // cambio para no conservar indefinidamente la primera traducción.
  const old=originalText.get(node);
  if (old && node.nodeValue!==old.rendered) originalText.set(node,{source:node.nodeValue,rendered:node.nodeValue});
  const source=originalText.get(node).source;
  const match=/^(\s*)([\s\S]*?)(\s*)$/.exec(source);
  if(!match)return;
  const recipe=renderedMessages.get(match[2]);
  const target=match[1]+(recipe?window.tr(recipe.msg,recipe.params):window.tr(match[2]))+match[3];
  if(node.nodeValue!==target)node.nodeValue=target;
  originalText.get(node).rendered=target;
 }
 function translateAttrs(element) {
  let originals=originalAttributes.get(element);
  if(!originals){originals={};originalAttributes.set(element,originals);}
  for(const attr of ['placeholder','title','aria-label']){
   if(!element.hasAttribute(attr))continue;
   if(!(attr in originals))originals[attr]=element.getAttribute(attr);
   const translated=window.tr(originals[attr]);
   if(element.getAttribute(attr)!==translated)element.setAttribute(attr,translated);
  }
 }
 function translateTree(root){
  if(root.nodeType===Node.TEXT_NODE){translateText(root);return;}
  if(root.nodeType!==Node.ELEMENT_NODE&&root.nodeType!==Node.DOCUMENT_NODE)return;
  if(root.nodeType===Node.ELEMENT_NODE)translateAttrs(root);
  const walker=document.createTreeWalker(root,NodeFilter.SHOW_TEXT|NodeFilter.SHOW_ELEMENT);
  while(walker.nextNode()){
   if(walker.currentNode.nodeType===Node.TEXT_NODE)translateText(walker.currentNode);
   else translateAttrs(walker.currentNode);
  }
 }
 async function choose(next){
  if(!supported.includes(next))return;
  const response=await fetch(window.gatewayURL('/admin/locales/'+encodeURIComponent(next)+'.json'),{cache:'no-store'});
  if(!response.ok)throw Error('No se pudo cargar el idioma');
  messages=await response.json();language=next;
  document.documentElement.lang=next;
  try{localStorage.setItem('headscale-gateway-language',next)}catch{}
  /** @type {HTMLSelectElement} */ (document.getElementById('ui-language')).value=next;
  translateTree(document);
 }
 async function loadSupported(){
  try {
   const response=await fetch(window.gatewayURL('/admin/locales'),{cache:'no-store'});
   if(!response.ok)return;
   const list=await response.json();
   if(!Array.isArray(list)||!list.includes('es'))return;
   supported=list;
   const levelResponse=await fetch(window.gatewayURL('/admin/locales/levels'),{cache:'no-store'});if(levelResponse.ok)window.gatewayLogLevels=await levelResponse.json();
   const select=/** @type {HTMLSelectElement} */ (document.getElementById('ui-language'));select.replaceChildren();
   for(const code of list){
    const option=document.createElement('option');option.value=code;
    try{option.textContent=new Intl.DisplayNames(['es'],{type:'language'}).of(code)}catch{option.textContent=code}
    select.append(option);
   }
   if(!supported.includes(language))language='es';
   select.value=language;
  }catch(e){console.warn('No se pudo consultar los idiomas disponibles',e)}
 }
 document.addEventListener('DOMContentLoaded',()=>{
  const selector=/** @type {HTMLSelectElement} */ (document.getElementById('ui-language'));selector.value=language;
  selector.addEventListener('change',()=>{void choose(selector.value).catch(console.error)});
  const observer=new MutationObserver(changes=>{
   for(const change of changes){
    if(change.type==='childList')for(const node of Array.from(change.addedNodes))translateTree(node);
    if(change.type==='characterData')translateTree(change.target);
   }
  });
  observer.observe(document.documentElement,{subtree:true,childList:true,characterData:true});
  void loadSupported().then(()=>choose(language)).catch(console.error);
 });
})();
