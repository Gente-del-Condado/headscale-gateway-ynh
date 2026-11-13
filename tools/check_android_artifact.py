#!/usr/bin/env python3
"""Comprueba identidad, firma y bibliotecas de una APK o un AAB ya construido."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import tempfile
import xml.etree.ElementTree as ET
import zipfile


def verify(artifact, sdk, ndk, java_home, abis, release=False, unsigned=False, bundletool=None):
    artifact, sdk, ndk = Path(artifact), Path(sdk), Path(ndk)
    env = dict(os.environ, JAVA_HOME=str(java_home)) if java_home else dict(os.environ)
    versions = sorted((sdk / 'build-tools').iterdir(), key=lambda p: tuple(int(x) for x in p.name.split('.') if x.isdigit()))
    tools = versions[-1]
    java = str(Path(java_home) / 'bin/java') if java_home else 'java'
    is_bundle = artifact.suffix == '.aab'
    if is_bundle:
        if not bundletool:
            raise ValueError('El AAB requiere --bundletool para validar su manifiesto')
        subprocess.run([java, '-jar', str(bundletool), 'validate', '--bundle='+str(artifact)], check=True, env=env)
        xml = subprocess.check_output([java, '-jar', str(bundletool), 'dump', 'manifest', '--bundle='+str(artifact), '--module=base'], env=env)
        manifest = ET.fromstring(xml)
        ns = '{http://schemas.android.com/apk/res/android}'
        info = {'package': manifest.attrib['package'], 'version': manifest.attrib[ns+'versionName'],
                'version_code': int(manifest.attrib[ns+'versionCode']),
                'target_sdk': int(manifest.find('uses-sdk').attrib[ns+'targetSdkVersion']),
                'debuggable': manifest.find('application').attrib.get(ns+'debuggable', 'false') == 'true'}
        with zipfile.ZipFile(artifact) as archive:
            if not any(name.startswith('META-INF/') and name.endswith(('.RSA', '.EC', '.DSA')) for name in archive.namelist()):
                raise ValueError('El AAB no está firmado')
        subprocess.run([str(Path(java_home) / 'bin/jarsigner') if java_home else 'jarsigner', '-verify', str(artifact)], check=True, env=env, stdout=subprocess.DEVNULL)
        keytool = str(Path(java_home) / 'bin/keytool') if java_home else 'keytool'
        certificate = subprocess.check_output([keytool, '-J-Duser.language=en', '-printcert', '-jarfile', str(artifact)], text=True, env=env)
        fingerprint = re.search(r'SHA256:\s*([0-9A-Fa-f:]+)', certificate)
        if not fingerprint or (release and 'CN=Android Debug' in certificate):
            raise ValueError('El AAB necesita un certificado de publicación verificable')
        info['certificate_sha256'] = fingerprint[1].replace(':', '').lower()
    else:
        badging = subprocess.check_output([str(tools / 'aapt'), 'dump', 'badging', str(artifact)], text=True, env=env)
        package = re.search(r"package: name='([^']+)' versionCode='([0-9]+)' versionName='([^']+)'", badging)
        info = {'package': package[1], 'version': package[3], 'version_code': int(package[2]),
                'target_sdk': int(re.search(r"targetSdkVersion:'([0-9]+)'", badging)[1]),
                'debuggable': 'application-debuggable' in badging}
        signature = subprocess.run([str(tools / 'apksigner'), 'verify', '--print-certs', str(artifact)], capture_output=True, text=True, env=env)
        if unsigned:
            if signature.returncode == 0:
                raise ValueError('La APK de F-Droid debe quedar sin firma para su servidor')
        elif signature.returncode:
            raise ValueError('La APK no tiene una firma verificable')
        else:
            fingerprint = re.search(r'certificate SHA-256 digest: ([0-9a-fA-F]+)', signature.stdout)
            info['certificate_sha256'] = fingerprint[1].lower() if fingerprint else None
            if release and 'CN=Android Debug' in signature.stdout:
                raise ValueError('No se publica una APK de producción con firma de depuración')
        subprocess.run([str(tools / 'zipalign'), '-c', '-P', '16', '4', str(artifact)], check=True, env=env)
    if release and (info['debuggable'] or info['package'] != 'org.headscalegateway.android' or info['target_sdk'] < 36):
        raise ValueError('Identidad, depuración o target SDK no válidos para la entrega Android')
    if release:
        version = (Path(__file__).resolve().parents[1] / 'VERSION').read_text().strip()
        major, minor, patch = map(int, version.split('.'))
        expected_code = (major * 1000000 + minor * 1000 + patch) * 10 + 4
        if info['version'] != version or info['version_code'] != expected_code:
            raise ValueError('Artefacto Android de otra versión: recompila antes de distribuir')
    readelf = next((ndk / 'toolchains/llvm/prebuilt').glob('*/bin/llvm-readelf'))
    prefix = 'base/lib/' if is_bundle else 'lib/'
    native = []
    with zipfile.ZipFile(artifact) as archive, tempfile.TemporaryDirectory(prefix='hgw-native-check-') as temporary:
        for abi in abis:
            names = [name for name in archive.namelist() if name.startswith(prefix+abi+'/') and name.endswith('.so')]
            if not any(name.endswith('/libheadscale_gateway.so') for name in names):
                raise ValueError('Falta el motor Go compartido: ' + abi)
            if not any(name.endswith('/libgateway-tunnel.so') for name in names):
                raise ValueError('Falta el adaptador JNI: ' + abi)
            for name in names:
                path = Path(temporary) / 'library.so'
                with archive.open(name) as source, path.open('wb') as output:
                    shutil.copyfileobj(source, output)
                headers = subprocess.check_output([str(readelf), '-lW', str(path)], text=True)
                loads = [line.split() for line in headers.splitlines() if line.strip().startswith('LOAD ')]
                if not loads or any(int(line[-1], 0) < 16384 for line in loads):
                    raise ValueError('Biblioteca sin alineación ELF de 16 KiB: ' + name)
                native.append(name)
    with artifact.open('rb') as stream:
        info['sha256'] = hashlib.file_digest(stream, 'sha256').hexdigest()
    info.update({'artifact': artifact.name, 'native_libraries': native, 'unsigned': unsigned})
    artifact.with_suffix(artifact.suffix+'.verification.json').write_text(json.dumps(info, indent=2)+'\n')
    return info


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('artifact', type=Path)
    parser.add_argument('--sdk', type=Path, required=True)
    parser.add_argument('--ndk', type=Path, required=True)
    parser.add_argument('--java-home', type=Path)
    parser.add_argument('--bundletool', type=Path)
    parser.add_argument('--release', action='store_true')
    parser.add_argument('--unsigned', action='store_true')
    args = parser.parse_args()
    print(json.dumps(verify(args.artifact, args.sdk, args.ndk, args.java_home,
                            ('arm64-v8a', 'x86_64'), args.release, args.unsigned, args.bundletool), indent=2))


if __name__ == '__main__':
    main()
