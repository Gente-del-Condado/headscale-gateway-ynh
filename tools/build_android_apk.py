#!/usr/bin/env python3
"""Construye APK y AAB con el motor común, firma por canal y avisos de terceros."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
from android_signing import load_environment
from check_android_artifact import verify as verify_artifact

ROOT = Path(__file__).resolve().parents[1]


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--sdk', required=True, type=Path, help='Android SDK instalado (API 36)')
    parser.add_argument('--ndk', required=True, type=Path, help='Android NDK 27.0.12077973')
    parser.add_argument('--gradle', default='gradle', help='Ejecutable de Gradle 8.13')
    parser.add_argument('--java-home', type=Path, help='JDK 17 para Gradle; si se omite utiliza JAVA_HOME')
    parser.add_argument('--arch', action='append', choices=('arm64-v8a','x86_64'), help='Se puede repetir; por defecto ambas')
    parser.add_argument('--release', action='store_true', help='Entrega estable: firma local, AAB de Play o APK sin firmar para F-Droid')
    parser.add_argument('--lab', action='store_true', help='Paquete de laboratorio aislado de la aplicación habitual')
    parser.add_argument('--channel', choices=('direct', 'fdroid', 'play'), default='direct')
    parser.add_argument('--signing-config', type=Path, help='Perfil privado de android_signing.py')
    parser.add_argument('--prepare-only', action='store_true', help='Prepara motor y avisos; no ejecuta Gradle')
    parser.add_argument('--bundletool', type=Path, help='bundletool oficial para validar el AAB de Play')
    args = parser.parse_args()
    if args.lab and args.release:
        parser.error('El laboratorio solo se construye como APK de pruebas')
    if args.channel != 'direct' and not args.release:
        parser.error('Los canales F-Droid y Play se preparan con --release')
    if args.channel == 'play' and not args.prepare_only and not args.bundletool:
        parser.error('Play requiere --bundletool para validar el AAB')
    signing = load_environment(args.signing_config) if args.signing_config and args.channel != 'fdroid' else {}
    if args.release and args.channel != 'fdroid' and not args.prepare_only and not all(signing.get(k) or os.environ.get(k) for k in ('HGW_ANDROID_KEYSTORE','HGW_ANDROID_STORE_PASSWORD','HGW_ANDROID_KEY_ALIAS','HGW_ANDROID_KEY_PASSWORD')):
        parser.error('La entrega necesita su propia firma; configura las cuatro variables HGW_ANDROID_*')
    architectures = args.arch or ['arm64-v8a','x86_64']
    for architecture in architectures:
        destination = ROOT / 'builds/android-jni' / architecture / 'libheadscale_gateway.so'
        subprocess.run([sys.executable,str(ROOT/'tools/build_android_engine.py'),'--ndk',str(args.ndk.resolve()),
                        '--arch',architecture,'--output',str(destination)],check=True)
    assets = ROOT/'android/src/main/assets'
    notices = assets/'licenses'
    notices.mkdir(parents=True,exist_ok=True)
    for name in ('LICENSE','NOTICE'):
        shutil.copy2(ROOT/name,notices/name)
    for name in ('go','go-toolchain'):
        shutil.copytree(ROOT/'src/third_party'/name,notices/'third_party'/name,dirs_exist_ok=True)
    shutil.copy2(ROOT/'android/third_party/TAILSOCKS-LICENSE',notices/'TAILSOCKS-LICENSE')
    vendor = ROOT/'android/third_party/hev-socks5-tunnel'
    for path in vendor.rglob('*'):
        if path.is_file() and path.name.upper().startswith(('LICENSE','COPYING')):
            target=notices/'hev'/path.relative_to(vendor); target.parent.mkdir(parents=True,exist_ok=True); shutil.copy2(path,target)
    (assets/'avisos.txt').write_text(
        'Headscale Gateway para Android\n\n'
        'Motor común del proyecto y adaptación Android propia.\n'
        'Código propio bajo AGPL-3.0-only. Fuentes y manual: '
        'https://gente-del-condado.github.io/headscale-gateway_ynh/ANDROID_USO.html\n'
        'La arquitectura móvil se ha contrastado con TailSocks. Se reutiliza su copia de HEV, '
        'con las licencias originales de todas las bibliotecas incluidas.\n'
        'Los avisos completos del motor Go y de las bibliotecas nativas están en assets/licenses/ dentro de esta APK.\n\n'
        +(ROOT/'android/third_party/TAILSOCKS-LICENSE').read_text()+'\n'+(vendor/'LICENSE').read_text())
    if args.prepare_only:
        print('Motor Go compartido y avisos preparados; no se ha creado una APK.')
        return
    # No se copia local.properties del proyecto original ni rutas personales.
    env=dict(os.environ,**signing,ANDROID_HOME=str(args.sdk.resolve()),ANDROID_SDK_ROOT=str(args.sdk.resolve()))
    if args.channel == 'fdroid':
        for key in ('HGW_ANDROID_KEYSTORE','HGW_ANDROID_STORE_PASSWORD','HGW_ANDROID_KEY_ALIAS','HGW_ANDROID_KEY_PASSWORD'):
            env.pop(key, None)
    if args.java_home: env['JAVA_HOME']=str(args.java_home.resolve())
    variant='release' if args.release else 'debug'
    is_bundle = args.channel == 'play'
    apk_dir=ROOT/f'android/build/outputs/{"bundle" if is_bundle else "apk"}/{variant}'
    # Zipflinger puede conservar huecos de binarios anteriores al actualizar
    # una APK incrementalmente. Empaqueta de nuevo sin limpiar los compilados.
    for previous in apk_dir.glob('*.aab' if is_bundle else '*.apk'): previous.unlink()
    subprocess.run([args.gradle,'--no-daemon','-p',str(ROOT/'android'),*(['-PlabBuild=true'] if args.lab else []),
                    '-PreleaseChannel='+args.channel,'-PtargetAbis='+','.join(architectures),
                    ':bundleRelease' if is_bundle else ':assembleRelease' if args.release else ':assembleDebug'],env=env,check=True)
    if is_bundle:
        bundles = list(apk_dir.glob('*.aab'))
        if len(bundles) != 1:
            raise SystemExit('Se esperaba un único AAB de Play')
        source = bundles[0]
    else:
        metadata=json.loads((apk_dir/'output-metadata.json').read_text())
        source=apk_dir/metadata['elements'][0]['outputFile']
    name = 'play.aab' if is_bundle else 'fdroid-unsigned.apk' if args.channel == 'fdroid' else ('laboratorio' if args.lab else variant)+'.apk'
    destination=ROOT/'builds/android'/('headscale-gateway-'+name)
    destination.parent.mkdir(parents=True,exist_ok=True); shutil.copy2(source,destination)
    checksum = hashlib.sha256(destination.read_bytes()).hexdigest()
    destination.with_suffix(destination.suffix+'.sha256').write_text(checksum+'  '+destination.name+'\n')
    verify_artifact(destination, args.sdk, args.ndk, args.java_home, architectures,
                    args.release, args.channel == 'fdroid', args.bundletool)
    print('Artefacto Android generado: '+str(destination))
    print('No se ha instalado ni publicado en ningún dispositivo o repositorio.')


if __name__=='__main__':
    main()
