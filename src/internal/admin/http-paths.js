'use strict';

// Obtiene el montaje de la URL de la página, nunca de cabeceras no confiables
// ni de ajustes persistidos. Admite /admin/ y /<montaje>/admin/panel.html.
(() => {
  const marker = '/admin/';
  const index = window.location.pathname.lastIndexOf(marker);
  const root = index < 0 ? '' : window.location.pathname.slice(0, index);
  window.gatewayURL = path => {
    if (!path.startsWith('/') || path.startsWith('//') || path.includes('..') || path.includes('\\')) {
      throw new Error('Invalid gateway path');
    }
    return root + path;
  };
})();
