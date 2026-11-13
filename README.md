# Headscale Gateway

Gateway de servicios privados para redes Headscale. Ejecuta un nodo VPN,
publica servicios con HTTPS y permite gestionar DNS, certificados, acceso y
copias cifradas desde un panel en español. Comparte el motor entre Linux,
Windows, macOS, Home Assistant, YunoHost y Android.

## Leer el manual

Manual web público: [YunoHost](https://gente-del-condado.github.io/headscale-gateway_ynh/)
y [Home Assistant](https://gente-del-condado.github.io/headscale-gateway-ha/).
Ambas webs muestran el mismo manual, generado desde las fuentes del proyecto.

Empieza por el [manual del proyecto](docs/index.md) y los
[primeros pasos](docs/PRIMEROS_PASOS.md). Incluye ejemplos, capturas y solución
de problemas. Para generar la web con búsqueda y navegación desde estas
mismas fuentes, sigue [Manual web y GitHub Pages](docs/MANUAL_WEB.md).

| Quiero… | Guía |
| --- | --- |
| Instalar el complemento de Home Assistant | [Tienda Mis-app](https://github.com/Gente-del-Condado/Mis-app) · [Instalación, Ingress y copias](packaging/homeassistant/DOCS.md) |
| Ver el Gateway como entidades de Home Assistant | [Integración de entidades](packaging/hacs/README.md) |
| Instalar en YunoHost | [Instalación, SSO y actualización](packaging/yunohost/doc/ADMIN.md) |
| Ejecutar el servidor directamente | [Manual de uso](docs/USO_COMPLETO.md) |
| Utilizar Android | [Aplicación Android](android/README.md) |
| Publicar el primer servicio | [Ejemplos completos](docs/EJEMPLOS.md) |
| Recuperar el acceso | [Procedimiento de recuperación](docs/RECUPERAR_ACCESO.md) |
| Modificar el código o continuar con una IA | [Guía de continuidad](AGENTS.md) y [contribución](CONTRIBUTING.md) |

## Organización

```text
headscale-gateway/
├── src/                    Motor Go, panel, cliente de bandeja y pruebas
│   ├── internal/           API, SQLite, PKI, copias y componentes compartidos
│   ├── desktop/            Cliente Electron
│   ├── locale/             Traducciones editables
│   ├── tools/              Utilidades Go y laboratorios visuales
│   └── third_party/        Inventario y avisos legales
├── android/                Aplicación, JNI y fuentes HEV necesarias
├── packaging/              Fuentes de instaladores y sus manuales
├── addons/                 Complemento Home Assistant generado e instalable
├── custom_components/      Integración de entidades Home Assistant
├── yunohost/               Paquete generado con ejecutables amd64 y arm64
├── tools/                  Compilación, verificaciones, empaquetado y tema web
├── tests/                  Contratos Python e integración con plataformas
├── docs/                   Manual y capturas
├── examples/               Configuraciones ficticias
├── AGENTS.md               Reglas y mapa para continuar el trabajo
└── mkdocs.yml              Navegación y construcción del manual web
```

El [mapa de componentes](docs/ESTRUCTURA.md) distingue fuentes, contenidos
generados y datos de ejecución. `addons/` y `yunohost/` forman parte de Git para
permitir instalar desde un clon privado. Se generan desde `src/` y `packaging/`.
La plantilla Home Assistant es `config.yaml.in`, que se convierte en
`config.yaml` dentro del complemento instalable.

## Preparar el desarrollo

Requisitos comunes del motor y sus verificaciones:

- Go **1.27.4**, según `src/go.mod`.
- Python **3.11 o posterior** para herramientas y MkDocs.
- Node.js **22** para comprobar JavaScript y trabajar en el cliente Electron.
- Git y las herramientas del sistema que necesite la plataforma de destino.

Desde la raíz del proyecto:

```bash
go -C src run -buildvcs=false . --help
python3 tools/check_repository.py
bash tools/verify.sh
```

En Windows utiliza `powershell -File tools/verify.ps1`. El módulo Go vive en
`src/`. Las dependencias se fijan en los archivos de módulo y lock existentes.
El motor ejecutable funciona independientemente del entorno de Node.js.

## Compilar y generar instaladores

```bash
# Ejecutables en bin/, fuera de las fuentes versionadas.
python3 tools/build_release.py --targets linux-amd64 windows-amd64

# Complemento e instalador incluidos en Git.
python3 tools/prepare_git_install.py
python3 tools/prepare_git_install.py --check

# ZIP de fuentes, con manifiesto de integridad.
python3 tools/repository_archive.py --output releases/headscale-gateway-fuentes.zip
```

`VERSION` identifica la versión común del motor. Para cambiarla utiliza
`python3 tools/set_version.py X.Y.Z` y regenera los instaladores. Las etiquetas
Git identifican commits y se gestionan según [Git y entregas](docs/MIGRACION_REPOSITORIO.md).

Android requiere JDK 17, Gradle 8.13, SDK API 36 y NDK 27.0.12077973. Su
[guía](android/README.md) contiene los comandos, permisos y ubicación de la APK.
Los resultados de compilación se guardan en `bin/`, `builds/` o `releases/`.

## Mantener una instalación

Utiliza el procedimiento de la plataforma, realiza una copia comprobada y
conserva la ubicación de sus datos. La configuración reside en SQLite y cada
nodo mantiene su identidad VPN. Las guías incluyen actualización, recuperación
y diferencias entre copiar ajustes y restaurar una identidad completa.

Los resultados comprobados y los recorridos que requieren equipos reales están
en [validación y límites](docs/VERIFICACION_FINAL.md). El [plan de pruebas](docs/PRUEBAS.md)
explica cómo repetir las verificaciones.

## Licencia y distribución

El código propio se distribuye bajo **GNU AGPL versión 3 únicamente**
(`AGPL-3.0-only`), con el alcance indicado en [NOTICE](NOTICE) y los términos
completos de [LICENSE](LICENSE). Se permite el uso, modificación y distribución,
también comercial, respetando sus condiciones. Los
avisos de dependencias están en [NOTICE](NOTICE) y en los inventarios de terceros.
Consulta [licencias](docs/LICENCIAS.md) antes de preparar una distribución.
