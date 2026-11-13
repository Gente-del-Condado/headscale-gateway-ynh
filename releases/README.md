# Entregas

Las entregas se generan en carpetas nuevas, sin datos de instalaciones. Esta
carpeta está ignorada por Git salvo este índice. La versión común es `VERSION`;
las etiquetas nuevas utilizan `vX.Y.Z` y nunca se mueve una etiqueta publicada.

```sh
python3 tools/build_release.py
python3 tools/package_release.py --output releases/1.4.7
python3 tools/package_platforms.py --output releases/plataformas-1.4.7
```

Los servidores cubren Linux, Windows y macOS en amd64/arm64. Para añadir los
clientes de bandeja Windows/Linux x64, ejecuta antes `npm ci` y
`npm run pack:all` desde `src/desktop/`. Cada escritorio conserva los fuentes
correspondientes de Electron/Chromium/FFmpeg, parches, avisos y sumas en `legal/`.
Estos fuentes ocupan varios GiB: no se retiran para reducir el tamaño de descarga.
El empaquetador rechaza servidores cuya versión o huella de fuentes sea antigua.

Android 1.4.7 utiliza el motor Go común: el APK directo lleva una firma estable,
el APK para F-Droid queda sin firmar y Play recibe un AAB de subida. Consulta
[los canales](../packaging/android/README.md). La variante `.lab` y los APK
`.dev` son solo de pruebas y no forman parte de la entrega estable.

Antes de adjuntar una entrega, comprueba `SHA256SUMS`, los informes Android,
los manifiestos y los resultados reales de las pruebas. Si el servidor limita
los adjuntos, crea un TAR de la entrega completa y usa `tools/split_release.py`;
publica también sus sumas y las instrucciones para unir las partes en orden.
No renombres ni dividas archivos a mano sin verificar su reconstrucción.

Las guías de plataforma y los paquetes instalables se mantienen en `packaging/`
y se generan en `addons/` y `yunohost/`. Los comandos locales de este índice no
publican ni instalan nada por sí solos. El flujo de main publica Mis-app,
YunoHost, los fuentes públicos y los manuales después de sus controles.
