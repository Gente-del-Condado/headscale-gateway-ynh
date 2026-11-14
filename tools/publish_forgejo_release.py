#!/usr/bin/env python3
"""Publica adjuntos preparados por el administrador con el token efímero del CI.

Solo habla con Forgejo por loopback. La carpeta y todos sus antecesores deben
ser de root y no escribibles por el runner. La etiqueta debe pertenecer a main
y mantener intactos los paquetes.
La release permanece en borrador hasta verificar por descarga todos los hashes.
"""
import argparse
import hashlib
import http.client
import json
import os
from pathlib import Path
import re
import secrets
import stat
import subprocess
from urllib.parse import quote

REPOSITORY = 'wolfbelmi88/headscale-gateway'
API = '/api/v1/repos/' + REPOSITORY


def private_tree(path, owner=0):
    for parent in (path, *path.parents):
        info = parent.lstat()
        if not stat.S_ISDIR(info.st_mode) or info.st_uid != owner or info.st_mode & 0o022:
            raise ValueError('La carpeta de publicación no está protegida frente a escritura')


def load_manifest(folder, tag, commit, owner=0):
    if not re.fullmatch(r'v[0-9]+\.[0-9]+\.[0-9]+', tag):
        raise ValueError('Etiqueta de versión no válida')
    folder = Path(folder).absolute()
    private_tree(folder, owner)
    manifest = folder / 'release-assets.json'
    info = manifest.lstat()
    if not stat.S_ISREG(info.st_mode) or info.st_uid != owner or info.st_mode & 0o022 or info.st_size > 1 << 20:
        raise ValueError('Manifiesto de publicación no protegido')
    data = json.loads(manifest.read_text())
    if data.get('tag') != tag or data.get('source_commit') != commit:
        raise ValueError('Los adjuntos no corresponden al commit etiquetado')
    if not isinstance(data.get('body'), str) or not data['body'].strip():
        raise ValueError('Faltan las notas de entrega revisadas')
    assets = data.get('assets')
    if not isinstance(assets, list) or not assets or len(assets) > 64:
        raise ValueError('Lista de adjuntos no válida')
    names = set()
    for asset in assets:
        name = asset['name']
        if not re.fullmatch(r'[A-Za-z0-9][A-Za-z0-9_.-]{0,180}', name) or name in names or name == manifest.name:
            raise ValueError('Nombre de adjunto no válido o repetido')
        names.add(name)
        file = folder / name
        info = file.lstat()
        if (not stat.S_ISREG(info.st_mode) or info.st_uid != owner or info.st_mode & 0o022
                or info.st_size != asset['bytes'] or not 0 < info.st_size <= 2_100_000_000
                or not re.fullmatch(r'[0-9a-f]{64}', asset['sha256'])):
            raise ValueError('Adjunto no protegido, tamaño no válido o enlace simbólico')
        with file.open('rb') as stream:
            if hashlib.file_digest(stream, 'sha256').hexdigest() != asset['sha256']:
                raise ValueError('La suma local de un adjunto no coincide')
    if {p.name for p in folder.iterdir()} != names | {manifest.name}:
        raise ValueError('La carpeta contiene archivos ajenos a la entrega')
    return data


class Forgejo:
    def __init__(self, token):
        if not token:
            raise ValueError('Falta el token temporal del trabajo')
        self.token = token

    def request(self, method, path, data=None, missing=False):
        connection = http.client.HTTPConnection('127.0.0.1', 6000, timeout=300)
        body = json.dumps(data).encode() if data is not None else None
        try:
            connection.request(method, path, body, {'Authorization': 'Bearer ' + self.token,
                               'Content-Type': 'application/json'})
            response = connection.getresponse()
            payload = response.read((1 << 20) + 1)
            if missing and response.status == 404:
                return None
            if not 200 <= response.status < 300 or len(payload) > 1 << 20:
                raise RuntimeError('Forgejo rechazó la operación: HTTP ' + str(response.status))
            return json.loads(payload) if payload else None
        finally:
            connection.close()

    def upload(self, release_id, file):
        boundary = 'hgw-' + secrets.token_hex(16)
        prefix = ('--' + boundary + '\r\nContent-Disposition: form-data; name="attachment"; filename="'
                  + file.name + '"\r\nContent-Type: application/octet-stream\r\n\r\n').encode()
        suffix = ('\r\n--' + boundary + '--\r\n').encode()
        connection = http.client.HTTPConnection('127.0.0.1', 6000, timeout=300)
        try:
            connection.putrequest('POST', API + '/releases/' + str(release_id) + '/assets')
            connection.putheader('Authorization', 'Bearer ' + self.token)
            connection.putheader('Content-Type', 'multipart/form-data; boundary=' + boundary)
            connection.putheader('Content-Length', str(len(prefix) + file.stat().st_size + len(suffix)))
            connection.endheaders()
            connection.send(prefix)
            with file.open('rb') as stream:
                while chunk := stream.read(1 << 20):
                    connection.send(chunk)
            connection.send(suffix)
            response = connection.getresponse()
            payload = response.read((1 << 20) + 1)
            if response.status != 201 or len(payload) > 1 << 20:
                raise RuntimeError('Forgejo rechazó un adjunto: HTTP ' + str(response.status))
            return json.loads(payload)
        finally:
            connection.close()

    def verify(self, remote, expected):
        if remote.get('name') != expected['name'] or remote.get('size') != expected['bytes'] or remote.get('type') != 'attachment':
            raise ValueError('Los metadatos del adjunto remoto no coinciden')
        # La API de Forgejo usa /attachments/UUID, sin el prefijo del repo.
        # El UUID procede del adjunto consultado en esta release. No se utiliza
        # browser_download_url ni se sigue una dirección externa de la respuesta.
        uuid = remote.get('uuid', '')
        if not re.fullmatch(r'[0-9a-f]{8}(?:-[0-9a-f]{4}){3}-[0-9a-f]{12}', uuid):
            raise ValueError('Identificador de adjunto inesperado')
        path = '/attachments/' + uuid
        connection = http.client.HTTPConnection('127.0.0.1', 6000, timeout=300)
        try:
            connection.request('GET', path, headers={'Authorization': 'Bearer ' + self.token})
            response = connection.getresponse()
            if response.status != 200:
                raise RuntimeError('No se puede comprobar la descarga: HTTP ' + str(response.status))
            digest, size = hashlib.sha256(), 0
            while chunk := response.read(1 << 20):
                size += len(chunk)
                if size > expected['bytes']:
                    raise ValueError('La descarga supera el tamaño esperado')
                digest.update(chunk)
            if size != expected['bytes'] or digest.hexdigest() != expected['sha256']:
                raise ValueError('La suma del adjunto descargado no coincide')
        finally:
            connection.close()


def publish(folder, tag, commit, client, draft_only=False, main_commit=None, recover_incomplete=False):
    data = load_manifest(folder, tag, commit)
    branch = client.request('GET', API + '/branches/main')
    remote_tag = client.request('GET', API + '/tags/' + quote(tag, safe=''), missing=draft_only)
    if (branch['commit']['id'] != (main_commit or commit) or (remote_tag is None and not draft_only)
            or (remote_tag is not None and remote_tag['commit']['sha'] != commit)):
        raise ValueError('La etiqueta o main no corresponden a los commits aprobados')
    release = client.request('GET', API + '/releases/tags/' + quote(tag, safe=''), missing=True)
    if release is None:
        release = client.request('POST', API + '/releases', {'tag_name': tag,
            'target_commitish': commit, 'name': 'Headscale Gateway ' + tag.removeprefix('v'),
            'body': data['body'], 'draft': True, 'prerelease': False})
    release_id = release['id']
    # Forgejo 17 devuelve aquí la lista completa y no pagina este recurso.
    # Repetir page=N vuelve a devolver los mismos adjuntos.
    existing = client.request('GET', API + '/releases/' + str(release_id) + '/assets')
    if not isinstance(existing, list) or len(existing) > 64:
        raise ValueError('Lista de adjuntos remotos no válida o excesiva')
    names = {asset['name']: asset for asset in existing}
    if len(names) != len(existing) or set(names) - {a['name'] for a in data['assets']}:
        raise ValueError('La release contiene adjuntos ajenos o repetidos; no se eliminan')
    verified = set()
    missing_names = {asset['name'] for asset in data['assets']} - set(names)
    if not release['draft'] and missing_names and recover_incomplete:
        # Forgejo convierte automáticamente un borrador en release al recibir
        # su etiqueta. Solo se recupera por petición explícita, después de
        # comprobar todos los adjuntos existentes contra el manifiesto protegido.
        # Una release completa o con archivos ajenos nunca cambia de estado aquí.
        for asset in data['assets']:
            if asset['name'] in names:
                client.verify(names[asset['name']], asset)
                verified.add(asset['name'])
                print('SHA256_REMOTO_OK: ' + asset['name'], flush=True)
        client.request('PATCH', API + '/releases/' + str(release_id), {'draft': True})
        release = client.request('GET', API + '/releases/' + str(release_id))
        if not release['draft'] or release['tag_name'] != tag:
            raise ValueError('No se ha confirmado la recuperación del borrador')
        print('BORRADOR_RECUPERADO: ' + tag, flush=True)
    for asset in data['assets']:
        if asset['name'] in verified:
            continue
        remote = names.get(asset['name'])
        if remote is None:
            if not release['draft']:
                raise ValueError('No se añaden archivos a una release ya publicada')
            remote = client.upload(release_id, Path(folder) / asset['name'])
        client.verify(remote, asset)
        print('SHA256_REMOTO_OK: ' + asset['name'], flush=True)
    if draft_only:
        if not release['draft']:
            raise ValueError('La entrega ya está publicada; no es un borrador')
        print('BORRADOR_VERIFICADO_SIN_PUBLICAR: ' + tag, flush=True)
        return
    if release['draft']:
        client.request('PATCH', API + '/releases/' + str(release_id), {'draft': False, 'body': data['body']})
    final = client.request('GET', API + '/releases/' + str(release_id))
    if final['draft'] or final['tag_name'] != tag:
        raise ValueError('No se ha confirmado la publicación')
    print('RELEASE_PUBLICADA_Y_VERIFICADA: ' + tag, flush=True)


# Un arreglo del publicador puede reintentar una etiqueta existente, sin moverla
# ni aceptar cambios posteriores en el motor, sus fuentes o los instaladores.
PUBLICATION_ONLY_PATHS = {
    '.forgejo/workflows/quality.yml', 'tools/publish_forgejo_release.py',
    'tests/test_forgejo_release.py', 'docs/MIGRACION_REPOSITORIO.md',
    'CONTENIDO.sha256',
}


def resolve_release_commit(tag, head, draft_only=False, repository=None):
    if not re.fullmatch(r'v[0-9]+\.[0-9]+\.[0-9]+', tag):
        raise ValueError('Etiqueta de versión no válida')
    prefix = ['git'] + (['-C', str(repository)] if repository is not None else [])
    def git(*args):
        return subprocess.check_output(prefix + list(args), text=True, stderr=subprocess.DEVNULL).strip()
    try:
        commit = git('rev-parse', '--verify', 'refs/tags/' + tag + '^{commit}')
    except subprocess.CalledProcessError:
        if draft_only:
            return head
        raise ValueError('La etiqueta debe existir antes de publicar') from None
    if commit != head:
        try:
            git('merge-base', '--is-ancestor', commit, head)
        except subprocess.CalledProcessError:
            raise ValueError('La etiqueta no pertenece al main aprobado') from None
        changed = set(git('diff', '--name-only', commit, head).splitlines())
        if changed - PUBLICATION_ONLY_PATHS:
            raise ValueError('Hay cambios de producto posteriores a la etiqueta; no se reutilizan los paquetes')
    return commit


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--tag', required=True)
    parser.add_argument('--staging', required=True)
    parser.add_argument('--draft-only', action='store_true', help='Preparar y verificar sin crear la etiqueta ni publicar')
    parser.add_argument('--recover-incomplete', action='store_true',
                        help='Recuperar como borrador una release incompleta tras verificar sus adjuntos existentes')
    args = parser.parse_args()
    if args.draft_only and args.recover_incomplete:
        parser.error('Recuperar una release incompleta requiere publicar, no solo preparar un borrador')
    head = subprocess.check_output(['git', 'rev-parse', 'HEAD'], text=True).strip()
    version = Path('VERSION').read_text().strip()
    if args.tag != 'v' + version:
        raise SystemExit('La etiqueta y VERSION no coinciden')
    commit = resolve_release_commit(args.tag, head, args.draft_only)
    publish(args.staging, args.tag, commit, Forgejo(os.environ.pop('HGW_RELEASE_TOKEN', '')),
            args.draft_only, main_commit=head, recover_incomplete=args.recover_incomplete)


if __name__ == '__main__':
    main()
