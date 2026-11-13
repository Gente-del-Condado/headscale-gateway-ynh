#!/usr/bin/env python3
"""Build source contracts, CLI options and supported integrations. No live secrets."""
import json,re,sys
from pathlib import Path
ROOT=Path(__file__).resolve().parent.parent
contracts={}
for p in sorted((ROOT/'src/internal/admin').glob('*.go')):
 if p.name.endswith('_test.go'):continue
 text=p.read_text()
 starts=list(re.finditer(r'mux.HandleFunc\("((?:GET|POST|PUT|DELETE|PATCH) /[^"]+)"',text))
 for i,m in enumerate(starts):
  snippet=text[m.start():starts[i+1].start() if i+1<len(starts) else len(text)]
  snippet=snippet.rstrip()
  fields=sorted(set(re.findall(r'`json:"([^",]+)',snippet)))
  query=sorted(set(re.findall(r'(?:Query\(\)\.Get|q\.Get)\("([^"]+)"',snippet)))
  contracts[m[1]]={'source':str(p.relative_to(ROOT)),'line':text[:m.start()].count('\n')+1,'fields':fields,'query':query,'code':snippet,'session':bool(re.search(r'requirePKISession|onlyAdmin\(|sessionAllowed|auth.Session\(',snippet)), 'admin': 'isAdminRequest' in snippet or 'gate(' in snippet}
# Las rutas de Forgejo comparten el control de sesión y dos campos de conexión.
for route,contract in contracts.items():
 if route.startswith('POST /api/v1/forgejo/'):
  contract['fields']=sorted(set(contract['fields']) | {'repository','token'})
  contract['session']=True
# Las rutas del portal registradas mediante bucles comparten este contrato.
for method,suffix in [('GET',''),('PUT',''),('POST','/sync'),('POST','/discover')]:
 contracts[method+' /api/v1/portal'+suffix]={'source':'src/internal/admin/routes_portal.go','line':19,'fields':[],'query':[],'code':(ROOT/'src/internal/admin/routes_portal.go').read_text(),'session':True,'admin':True}
# Los campos proceden del código; no forman un esquema JSON de petición. Incluyen respuestas.
cli=[]
for p in [ROOT/'src/internal/bootstrap/options.go',ROOT/'src/internal/cli/cli.go']:
 for m in re.finditer(r'fs\.(?:String|Bool|Int)Var\(&\w+\.\w+, "([^"]+)", ([^,]+), "([^"]+)"\)',p.read_text()):
  cli.append({'name':'--'+m[1],'default':m[2],'description':m[3],'source':str(p.relative_to(ROOT)),'context':'startup' if 'bootstrap' in str(p) else 'diagnostics'})
commands=[
 ('version','Versión y plataforma','./headscale-gateway version'),
 ('doctor','Diagnóstico local sin iniciar la VPN','./headscale-gateway doctor --db /ruta/gateway.sqlite'),
 ('logs status','Estado de los registros','./headscale-gateway logs status --dir /ruta/logs'),
 ('logs tail','Últimas líneas del registro','./headscale-gateway logs tail --dir /ruta/logs --channel gateway --lines 50'),
 ('logs follow','Seguir nuevas líneas; Ctrl+C para terminar','./headscale-gateway logs follow --dir /ruta/logs --channel admin'),
 ('--help','Ayuda de arranque','./headscale-gateway --help'),
 ('--db','Arrancar con una base de datos','./headscale-gateway --db /ruta/gateway.sqlite')]
external=[]
for p in sorted((ROOT/'src/internal/network').glob('*.go')):
 if p.name.endswith('_test.go'):continue
 for m in re.finditer(r'a.call\(ctx, "(GET|POST|PUT|DELETE)", (.*?), (?:nil|map\[|req)',p.read_text()):
  external.append({'name':'Headscale','method':m[1],'endpoint':'/api/v1/'+''.join((part[1:-1] if part.startswith(chr(34)) else '{'+re.sub(r'url\.(?:PathEscape|QueryEscape)\((\w+)\)',r'\1',part)+'}') for part in m[2].split('+')),'source':str(p.relative_to(ROOT))})
external.append({'name':'Headscale','method':'GET','endpoint':'/api/v1/node','source':'src/internal/network/headscale.go'})
for action in ['get','add','update','delete']:
 external.append({'name':'Technitium','method':'GET','endpoint':'/api/zones/records/'+action,'source':'dns_technitium.go'})
for method,path in [('GET',''),('POST',''),('PATCH','/{record_id}'),('DELETE','/{record_id}')]:
 external.append({'name':'Cloudflare','method':method,'endpoint':'/client/v4/zones/{zone_id}/dns_records'+path,'source':'dns_cloudflare.go'})
integrations=[
 {'name':'Headscale','description':'Inventario, usuarios, claves, etiquetas, rutas y ACL mediante REST. La clave de Headscale es independiente del token del Gateway. Las operaciones disponibles dependen de la versión del servidor.','source':'src/internal/network','docs':'https://headscale.net/stable/ref/api/'},
 {'name':'Technitium','description':'Consultar, crear, actualizar y borrar registros de la zona configurada. Su API utiliza GET también para modificaciones. El token se utiliza solo en el servidor.','source':'dns_technitium.go','docs':'https://github.com/TechnitiumSoftware/DnsServer/blob/master/APIDOCS.md'},
 {'name':'Cloudflare','description':'Registros DNS de la zona configurada; publicación sin proxy. ACME utiliza credenciales de su proveedor configurado.','source':'dns_cloudflare.go','docs':'https://developers.cloudflare.com/api/resources/dns/subresources/records/'},
 {'name':'Tailscale / tsnet','description':'SDK y LocalClient internos para estado, preferencias, login y diagnósticos. No se expone un proxy genérico de la API local de Tailscale.','source':'src/internal/network; tailscale.go','docs':'https://tailscale.com/kb/1244/tsnet'},
 {'name':'ACME / CertMagic','description':'Emisión y renovación TLS mediante el cliente ACME y desafío DNS. No hay una API administrativa ACME pública en el Gateway.','source':'certificates.go; go.mod','docs':'https://github.com/caddyserver/certmagic'},
 {'name':'OIDC / LDAP','description':'Autenticación y asignación explícita de roles. OIDC usa discovery, autorización, token y JWKS del proveedor; LDAP es un protocolo de directorio, no REST.','source':'src/internal/admin/auth_oidc.go; internal/admin/auth_ldap.go','docs':'https://openid.net/specs/openid-connect-discovery-1_0.html'},
 {'name':'Federación','description':'Catálogo HTTPS versionado y CA pública opcional. Solo peers aprobados, TLS verificado y ACL; ninguna credencial administrativa se transmite al peer.','source':'src/internal/portal; internal/pki','docs':''},
 {'name':'Repositorio de plantillas','description':'Descarga HTTPS de catálogos JSON desde la URL configurada; puede alojarse en Forgejo o GitHub. No utiliza una API de administración de repositorios.','source':'src/internal/templatelib','docs':''}]
examples={
 'POST /api/v1/network/ping':{'node_id':'1','mode':'tsmp'},
 'PUT /api/v1/config':{'changes':{'admin.enabled':'true'}},
 'POST /api/v1/network/admin/policy/parse':{'policy':'{"grants":[]}'},
 'PUT /api/v1/network/admin/policy':{'policy':'{"grants":[]}','revision':'REEMPLAZAR_POR_REVISION_LEIDA'},
 'PUT /api/v1/pki/publication':{'enabled':False},
 'PUT /api/v1/pki/helper':{'enabled':False},
 'POST /api/v1/auth/login':{'username':'USUARIO','password':'CONTRASEÑA','totp':'','remember':False},
 'POST /api/v1/auth/mfa/confirm':{'code':'000000'},
 'POST /api/v1/auth/mfa/disable':{'code':'000000'},
 'POST /api/v1/auth/bootstrap':{'code':'CODIGO_LOCAL'},
 'POST /api/v1/auth/recovery/rotate':{'confirm':'ROTAR'},
 'PUT /api/v1/network/gateway/preferences':{'accept_routes':False,'accept_dns':False,'advertise_routes':[]},
}
result={'contracts':contracts,'commands':[dict(name=a,description=b,example=c) for a,b,c in commands],'options':cli,'integrations':integrations,'external':external,'examples':examples}
output=json.dumps(result,ensure_ascii=False,indent=2)+'\n'
destination=ROOT/'src/internal/admin/reference-data.json'
if '--check' in sys.argv:
 if not destination.exists() or destination.read_text()!=output:raise SystemExit('Reference is stale: run python3 tools/reference_catalog.py')
else:destination.write_text(output)
print(f'Reference: {len(contracts)} source contracts, {len(cli)} CLI flags, {len(external)} external operations')
