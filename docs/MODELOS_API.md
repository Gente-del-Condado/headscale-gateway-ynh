# Modelos de entrada de la API

Los contratos del código y el índice vivo de Sistema → API / CLI prevalecen ante diferencias. Este apéndice describe los modelos principales; los campos del portal están en `src/internal/portal/model.go`.

### Modelos de entrada y validaciones específicas

**Servicios:** objeto `Service` con `name` obligatorio, `enabled` opcional, `backend`, `description`, `template` y `proxy`. El `PUT` rechaza `name` distinto del `{name}` de ruta. Los bodies de JSON que contengan propiedades desconocidas son rechazados con 400. El límite general del decodificador JSON es **128 KiB**; algunos endpoints usan decodificadores con límites específicos.

**Plantillas de proxy:** objetos `TemplateRecord` con `name`, `category`, `description`, `backend_hint` y `proxy`. La exportación/importación utiliza un bundle `headscale-gateway-templates`, versión 1, y una opción `overwrite`. El repositorio remoto almacena `url` y `allow_private`; este último debe configurarse con precaución y no debe habilitarse para URL arbitrarias sin control.

**Política ACL de Headscale:** `PUT /api/v1/network/admin/policy` exige `policy` (cadena **huJSON**, máximo 100 KiB) y `revision` procedente de una lectura previa (`SHA-256` o `missing`). El programa vuelve a consultar la política antes de escribir; **esto no constituye un compare-and-swap atómico del lado de Headscale**. `POST .../parse` transforma huJSON en un objeto para validación/visualización, **sin aplicar cambios**. Un objeto ACL vacío podría tener semántica de autorización amplia en Headscale: no lo publiques sin revisar.

**Acciones administrativas de Headscale:** el campo `action` de `POST /api/v1/network/admin/action` únicamente admite:

| Familia | Valores de `action` | Campos pertinentes |
|---|---|---|
| Nodos | `node.rename`, `node.expire`, `node.delete` | `id`, `name` para renombrar |
| Etiquetas/rutas | `node.tags`, `node.routes` | `id`, `tags` (`tag:...`) o `routes` (CIDR) |
| Usuarios | `user.create`, `user.rename`, `user.delete` | `name` para crear, `id` y `name` para renombrar, `id` para borrar |
| Claves de preautorización | `key.create`, `key.expire`, `key.delete` | `user`, `reusable`, `ephemeral`, `hours`, `tags` al crear; `id` al caducar/borrar |

Se validan los IDs de nodos, nombres, CIDR y etiquetas. La URL de destino de Headscale y los métodos HTTP se construyen internamente: **no** son parámetros elegidos por el navegador. La administración depende de la API y versión de Headscale desplegada; una respuesta `502` puede indicar error o incompatibilidad de la API del VPS.

**Tokens delegados:** `POST /api/v1/api/tokens` admite solo `viewer` u `operator`, un array no vacío de scopes permitidos, y `days` entre 1 y 365. El token secreto se devuelve al crearlo; `GET /api/v1/api/tokens` solo devuelve metadatos. Un token `viewer` no se convierte en administrador por declarar `identity` o `settings`; esos scopes tampoco son permitidos al crear tokens delegados.

**Auditoría:** `GET /api/v1/audit` **sin query string** mantiene compatibilidad devolviendo un array de los últimos 100 eventos. Con **cualquier query string** utiliza respuesta paginada. `page` es como mínimo 1 y como máximo 100000; `size` acepta **10, 25 o 50** (otros valores se normalizan a 10). `sort` y `order` pasan por la implementación controlada del almacén, no son SQL libre.

**MFA:** TOTP se prepara con `mfa/setup` y se confirma mediante `mfa/confirm`. La rotación de recuperación exige exactamente `{"confirm":"ROTAR"}` y sesión interactiva con rol admin. **No puede iniciarse solamente con el token Bearer de recuperación.**

**Copias:** `components` admite `sqlite`, `acl`, `dns`, `identity` y `full` como booleanos. Para SQLite se incluye la clave maestra necesaria. La retención `keep` es de **1 a 30** y la frecuencia `interval_hours` admite **0, 6, 12, 24 o 168**. `0` deshabilita la copia programada. La subida `.hgw` usa `multipart/form-data` (no JSON) con campos exactos `archive` y `password`; la implementación establece un máximo de **50 GiB** para el cuerpo HTTP, pero puede necesitar mucho espacio temporal para validar/descomprimir. Una petición de restauración integral responde 202 e inicia una preparación/reinicio, **no significa que la restauración haya terminado**: consulta `/api/v1/backups/restore/status`.

**Forgejo:** las cinco rutas requieren sesión interactiva de administrador y CSRF. Son `POST`, usan JSON de hasta 16 KiB y reciben `repository` (URL HTTPS del repositorio privado) y `token` en el cuerpo. La lista y las importaciones funcionan con `read:repository`; las exportaciones requieren `write:repository`.

| Ruta bajo `/api/v1/forgejo` | Campos adicionales | Resultado |
| --- | --- | --- |
| `/list` | `page`, entero desde 1 | `items` con `tag`, `name`, `size`, `sha256` y `created_at`; `more` indica si puede haber otra página. |
| `/profile/export` | `password`, `secrets` (booleano) | 201 y ficha del perfil cifrado guardado. |
| `/profile/import` | `tag`, `password`, `apply` (booleano) | Con `apply: false`, devuelve `keys` sin valores. Con `apply: true`, aplica el mismo perfil y devuelve `imported` y `restart_required`. |
| `/backup/export` | `id` de una copia local | 201 y ficha de la copia cifrada guardada. |
| `/backup/import` | `tag`, `password` | 201 y ficha de la copia verificada en el inventario local. La restauración se realiza aparte. |

**PKI:** `{fingerprint}` es la huella SHA-256 de un certificado. Alta y aprobación son operaciones distintas. `POST .../approve` y las operaciones `install/uninstall` comprueban la coincidencia con la huella completa en el body. Instalar una CA requiere permisos del sistema; una respuesta 409 puede significar que la instalación fue rechazada por permisos. `DELETE /api/v1/pki/peers/{fingerprint}` revoca primero una CA aprobada; una llamada posterior, estando ya no aprobada, permite eliminarla del inventario.

