'use strict';

// El índice se genera con MkDocs. La búsqueda funciona sin servidor ni CDN.
const search = document.getElementById('manual-search');
const results = document.getElementById('search-results');
const status = document.getElementById('search-status');
const normalize = text => text.normalize('NFD').replace(/[\u0300-\u036f]/g, '').toLowerCase();
const entries = (window.HGW_SEARCH || []).map(entry => ({
  ...entry,
  titleSearchable: normalize(entry.title),
  searchable: normalize(entry.title + ' ' + entry.text),
}));
let searchTimer;

function showResults() {
  const query = normalize(search.value.trim());
  results.replaceChildren();
  results.hidden = query.length < 2;
  status.textContent = '';
  if (query.length < 2) return;

  const words = query.split(/\s+/);
  const matches = entries.filter(entry => words.every(word => entry.searchable.includes(word)));
  // Prioriza el título para encontrar una plataforma aunque aparezca en muchas guías.
  matches.sort((a, b) => Number(b.titleSearchable.includes(query)) - Number(a.titleSearchable.includes(query)));
  status.textContent = matches.length ? `${matches.length} páginas encontradas${matches.length > 12 ? "; se muestran las primeras 12" : ""}` : 'No hay resultados. Prueba con otra palabra.';
  for (const entry of matches.slice(0, 12)) {
    const item = document.createElement('li');
    const link = document.createElement('a');
    link.href = `${document.body.dataset.base}/${entry.url}`;
    link.textContent = entry.title;
    item.append(link);
    results.append(item);
  }
}

// Se retrasa el anuncio para no interrumpir cada tecla con el lector de pantalla.
search.addEventListener('input', () => {
  clearTimeout(searchTimer);
  searchTimer = setTimeout(showResults, 200);
});

// El menú conserva sus controles nativos y sigue siendo visible sin JavaScript.
const menu = document.getElementById('manual-menu');
const mobile = window.matchMedia('(max-width: 960px)');
function adjustMenu() { menu.open = !mobile.matches; }
adjustMenu();
mobile.addEventListener('change', adjustMenu);
