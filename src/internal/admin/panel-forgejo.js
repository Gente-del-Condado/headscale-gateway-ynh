'use strict';
(() => {
  const element = id => document.getElementById('forgejo-' + id);
  const input = id => /** @type {HTMLInputElement} */ (element(id));
  const form = /** @type {HTMLFormElement} */ (element('form'));
  const items = /** @type {HTMLSelectElement} */ (element('items'));
  let page = 1;
  let backupID = '';
  let busy = false;

  function hidePreview() {
    element('preview').hidden = true;
    element('preview-keys').replaceChildren();
  }

  function resetList() {
    hidePreview();
    element('next').hidden = true;
    element('previous').hidden = true;
    element('recovery').hidden = true;
    items.replaceChildren();
    input('password').value = '';
  }

  async function perform(operation) {
    if (busy || !input('repository').reportValidity() || !input('token').reportValidity()) return;
    const credentials = {repository: input('repository').value.trim(), token: input('token').value.trim()};
    const controls = /** @type {(HTMLInputElement | HTMLButtonElement | HTMLSelectElement)[]} */ (Array.from(form.elements));
    busy = true;
    controls.forEach(control => { control.disabled = true; });
    element('message').textContent = window.tr('Conectando con Forgejo…');
    try {
      await operation(credentials);
    } catch (error) {
      element('message').textContent = error.message;
    } finally {
      busy = false;
      controls.forEach(control => { control.disabled = false; });
    }
  }

  function list(requestedPage) {
    return perform(async credentials => {
      resetList();
      const result = await api('forgejo/list', 'POST', {...credentials, page: requestedPage});
      page = requestedPage;
      for (const entry of result.items) {
        const option = document.createElement('option');
        option.value = entry.tag;
        option.textContent = `${entry.name} · ${new Date(entry.created_at).toLocaleString()} · ${(entry.size / 1048576).toFixed(2)} MiB · ${entry.sha256.slice(0, 12)}`;
        items.append(option);
      }
      element('recovery').hidden = !result.items.length;
      element('next').hidden = !result.more;
      element('previous').hidden = page === 1;
      element('message').textContent = result.items.length
        ? window.tr('Selecciona el archivo que quieres recuperar.')
        : window.tr('Esta página no contiene archivos cifrados del Gateway.');
    });
  }

  form.addEventListener('submit', event => {
    event.preventDefault();
    void list(1);
  });
  element('previous').addEventListener('click', () => void list(page - 1));
  element('next').addEventListener('click', () => void list(page + 1));
  for (const field of ['repository', 'token']) {
    input(field).addEventListener('input', resetList);
  }
  document.getElementById('logout').addEventListener('click', () => {
    form.reset();
    resetList();
    element('local').hidden = true;
    element('profile').hidden = true;
    element('message').textContent = '';
    backupID = '';
  });
  items.addEventListener('change', hidePreview);
  input('password').addEventListener('input', hidePreview);

  document.getElementById('config-portable-forgejo').addEventListener('click', () => {
    document.querySelector('[data-tab="backups"]').click();
    element('profile').hidden = false;
    input('export-secrets').checked = /** @type {HTMLInputElement} */ (document.getElementById('config-portable-secrets')).checked;
    element('title').focus();
  });

  document.addEventListener('gateway-forgejo-backup', event => {
    backupID = /** @type {CustomEvent<string>} */ (event).detail;
    element('local-selection').textContent = window.tr('Copia seleccionada: {p0}', {p0: backupID});
    element('local').hidden = false;
    element('title').focus();
  });

  element('profile-export').addEventListener('click', () => void perform(async credentials => {
    const password = input('export-password').value;
    if (password.length < 12) throw Error(window.tr('La contraseña debe tener al menos 12 caracteres'));
    const secrets = input('export-secrets').checked;
    if (secrets && !confirm(window.tr('El archivo cifrado incluirá claves de API de Headscale y DNS. ¿Continuar?'))) {
      element('message').textContent = '';
      return;
    }
    try {
      await api('forgejo/profile/export', 'POST', {...credentials, password, secrets});
      element('message').textContent = window.tr('Perfil cifrado guardado en Forgejo.');
    } finally {
      input('export-password').value = '';
    }
  }));

  element('backup-export').addEventListener('click', () => void perform(async credentials => {
    await api('forgejo/backup/export', 'POST', {...credentials, id: backupID});
    element('message').textContent = window.tr('Copia cifrada guardada en Forgejo.');
  }));

  element('recover').addEventListener('click', () => void perform(async credentials => {
    hidePreview();
    const tag = items.value;
    const password = input('password').value;
    if (!tag || !password) throw Error(window.tr('Archivo y contraseña requeridos.'));
    if (tag.startsWith('hgw-backup-')) {
      try {
        await api('forgejo/backup/import', 'POST', {...credentials, tag, password});
        element('message').textContent = window.tr('Copia verificada. Puedes restaurarla desde la lista de copias locales.');
        document.dispatchEvent(new Event('gateway-forgejo-imported'));
      } finally {
        input('password').value = '';
      }
      return;
    }
    const result = await api('forgejo/profile/import', 'POST', {...credentials, tag, password, apply: false});
    for (const key of result.keys) {
      const item = document.createElement('li');
      item.textContent = key;
      element('preview-keys').append(item);
    }
    element('preview').hidden = false;
    element('message').textContent = window.tr('Revisa las opciones antes de aplicar el perfil.');
  }));

  element('apply').addEventListener('click', () => {
    if (element('preview').hidden || !confirm(window.tr('¿Aplicar los ajustes del perfil a esta instalación?'))) return;
    void perform(async credentials => {
      try {
        const result = await api('forgejo/profile/import', 'POST', {
          ...credentials, tag: items.value, password: input('password').value, apply: true
        });
        element('message').textContent = window.tr('Importadas {p0} opciones. Reinicia el Gateway para aplicarlas.', {p0: result.imported});
        window.markRestartRequired?.();
      } finally {
        hidePreview();
        input('password').value = '';
      }
    });
  });
})();
