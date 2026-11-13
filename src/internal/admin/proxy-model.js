'use strict';
// API independiente del DOM para compartir la serialización con pruebas Node.
// Mantiene las claves ajenas al editor y no confunde "false", 0 o "0s" con ausencia.
const GatewayProxyModel = (() => {
  const isObject = v => v !== null && typeof v === 'object' && !Array.isArray(v);
  const clone = v => JSON.parse(JSON.stringify(v ?? {}));
  const pathGet = (obj, path) => path.split('.').reduce((v, p) => isObject(v) ? v[p] : undefined, obj);
  function pathSet(obj, path, value) {
    const names = path.split('.');
    let node = obj;
    for (const name of names.slice(0, -1)) {
      if (!isObject(node[name])) node[name] = {};
      node = node[name];
    }
    if (value === undefined) delete node[names.at(-1)];
    else node[names.at(-1)] = value;
    // Elimina objetos vacíos creados por el editor (ej. tls: {}).
    for (let n = names.length - 1; n > 0; n--) {
      const parentPath = names.slice(0, n - 1);
      const parent = parentPath.length ? pathGet(obj, parentPath.join('.')) : obj;
      const child = parent[names[n - 1]];
      if (isObject(child) && !Object.keys(child).length) delete parent[names[n - 1]];
      else break;
    }
  }
  function parseField(kind, value) {
    if (value === '') return undefined; // Herencia de la plantilla.
    if (kind === 'boolean') {
      if (value !== 'true' && value !== 'false') throw Error('Booleano inválido');
      return value === 'true';
    }
    if (kind === 'choice') return value;
    if (kind === 'integer' || kind === 'select') {
      if (!/^\d+$/.test(value)) throw Error('Número inválido');
      return Number(value);
    }
    if (kind === 'duration' && !/^(?:\d+(?:\.\d+)?(?:ns|us|µs|ms|s|m|h))+$/i.test(value)) {
      throw Error('Duración inválida; utiliza, por ejemplo, 30s, 2m o 1h');
    }
    return value;
  }
  function encode(original, fields, values, headers) {
    const result = clone(original);
    if (!isObject(result)) throw Error('Proxy no es un objeto');
    for (const field of fields) {
      if (!Object.prototype.hasOwnProperty.call(values, field.key)) continue;
      const value = parseField(field.kind, String(values[field.key]).trim());
      if ((field.kind === 'select' || field.kind === 'choice') && value !== undefined &&
          !field.options?.some(o => field.kind === 'choice' ? o.value === value : Number(o.value) === value)) {
        throw Error('Opción no soportada: ' + field.key);
      }
      pathSet(result, field.key, value);
    }
    for (const key of ['request_headers', 'response_headers']) {
      if (!Object.prototype.hasOwnProperty.call(headers, key)) continue;
      const map = headers[key];
      if (!isObject(map)) throw Error('Formato de cabeceras inválido');
      if (Object.keys(map).length) result[key] = map;
      else delete result[key];
    }
    return result;
  }
  const stringify = value => value === undefined || value === null ? '' : String(value);
  return {clone, pathGet, encode, stringify};
})();
