#!/usr/bin/env python3
"""Comprueba VPN y servicios en un Android de laboratorio; modifica sus datos de pruebas.

Requiere consentimiento VPN previo, ambas APK instaladas, Headscale temporal
accesible desde Android y desde 127.0.0.1:18480 en el equipo de pruebas.
Nunca utilizar con una instalación de producción.
"""
import subprocess,json,urllib.request,pathlib,time,shlex,argparse,base64
parser=argparse.ArgumentParser(description=__doc__)
parser.add_argument('--adb',default='adb')
parser.add_argument('--package',default='org.headscalegateway.android.lab')
parser.add_argument('--vpn',action='store_true',help='Comprueba además el TUN; exige consentimiento Android previo')
parser.add_argument('--functional',action='store_true',help='Escribe y restaura datos solo en la APK .lab')
parser.add_argument('--api-key-file',type=pathlib.Path)
parser.add_argument('--control-ca-file',type=pathlib.Path)
parser.add_argument('--peer-control-url',default='http://127.0.0.1:18480')
parser.add_argument('--serial',required=True)
parser.add_argument('--headscale-url',required=True)
parser.add_argument('--key-file',required=True,type=pathlib.Path)
parser.add_argument('--peer',required=True,type=pathlib.Path)
parser.add_argument('--peer-data',required=True,type=pathlib.Path)
parser.add_argument('--log-dir',required=True,type=pathlib.Path)
args=parser.parse_args()
if not args.package.endswith('.lab'):
 parser.error('Utiliza el paquete .lab: el ejecutor modifica y restaura datos de laboratorio')
if args.functional and not args.headscale_url.startswith('https://'):
 parser.error('Las pruebas funcionales requieren un Headscale de laboratorio con HTTPS válido')
args.log_dir.mkdir(parents=True,exist_ok=True,mode=0o700)
adb=args.adb;serial=args.serial
log=(args.log_dir/'android.log');log.touch(mode=0o600,exist_ok=True)
cmd=[adb,'-s',serial,'shell','am','instrument','-w','-e','headscale',args.headscale_url,'-e','vpn',str(args.vpn).lower(),'-e','authkey',args.key_file.read_text().strip(),args.package+'.test/org.headscalegateway.android.SmokeInstrumentation']
if args.functional:cmd+=['-e','functional','true']
# am espera las opciones antes del componente.
component=cmd.pop(cmd.index(args.package+'.test/org.headscalegateway.android.SmokeInstrumentation'))
if args.api_key_file:cmd+=['-e','apikey',args.api_key_file.read_text().strip()]
if args.control_ca_file:cmd+=['-e','ca',args.control_ca_file.read_text()]
cmd.append(component)
remote='/data/local/tmp/hgw-lab-output'
subprocess.run([adb,'-s',serial,'shell','am','force-stop',args.package],check=True)
time.sleep(2)
subprocess.run([adb,'connect',serial],capture_output=True,timeout=15)
command='umask 077; : > '+remote+'; '+shlex.join(cmd[4:])+' >> '+remote+' 2>&1 < /dev/null &'
try:
 launched=subprocess.run([adb,'-s',serial,'shell',command],capture_output=True,text=True,timeout=15)
except subprocess.TimeoutExpired:
 raise RuntimeError('ADB no respondió al iniciar el laboratorio') from None
if launched.returncode:raise RuntimeError('No se pudo iniciar la instrumentación; comprueba ADB')
def lines():
 seen=0;deadline=time.monotonic()+300
 while time.monotonic()<deadline:
  result=subprocess.run([adb,'-s',serial,'shell','cat',remote],capture_output=True,text=True,timeout=15)
  if result.returncode:
   subprocess.run([adb,'connect',serial],capture_output=True,timeout=15);time.sleep(1);continue
  rows=result.stdout.splitlines()
  for row in rows[seen:]:yield row
  seen=len(rows)
  if any('INSTRUMENTATION_CODE:' in row for row in rows):break
  time.sleep(1)
peer=None;checked=False;last='';success=False
try:
 with log.open('w') as out:
  for line in lines():
   if 'lab=' not in line:
    out.write(line+"\n");out.flush();continue
   v=json.loads(line.split('lab=',1)[1]);phase=v['phase']
   if phase in ('archive-export','profile-export','selected-export'):
    suffix='hgwcfg' if phase=='profile-export' else 'hgw'
    (args.log_dir/(('android-selected.' if phase=='selected-export' else 'android.')+suffix)).write_bytes(base64.b64decode(v.pop('data')))
    line='INSTRUMENTATION_STATUS: lab='+json.dumps(v)
   out.write(line+"\n");out.flush()
   if phase!=last:print('Fase Android:',phase,flush=True);last=phase
   if v.get('detail'):print('  '+v['detail'],flush=True)
   if phase in ('connected','vpn') and peer is None:
    ip=v['state']['ips'][0];ca=args.log_dir/'gateway-ca.crt';ca.write_text(v['ca_pem']);peerlog=(args.log_dir/'peer.log').open('w')
    peer=subprocess.Popen([str(args.peer.resolve()),'--data',str(args.peer_data.resolve()),'--key-file',str(args.key_file.resolve()),'--gateway',ip,'--ca-file',str(ca),'--service-host',v['service_host'],'--control-url',args.peer_control_url,*(['--control-ca-file',str(args.control_ca_file)] if args.control_ca_file else []),'--serve'],stdout=peerlog,stderr=subprocess.STDOUT)
   if phase=='ready' and not checked:
    deadline=time.monotonic()+60
    while time.monotonic()<deadline:
     try:
      with urllib.request.urlopen('http://127.0.0.1:18482/fetch',timeout=15) as r:
       assert r.status==200
      print('PASS: servicio HTTPS del teléfono desde otro nodo VPN',flush=True);break
     except Exception:time.sleep(1)
    else:raise AssertionError('No se pudo abrir el servicio HTTPS desde el segundo nodo')
    if args.vpn:
     peerip=(args.log_dir/'peer.log').read_text().splitlines()[0]
     request='GET / HTTP/1.1\r\nHost: laboratorio\r\nConnection: close\r\n\r\n'
     r=subprocess.run([adb,'-s',serial,'shell','toybox','nc','-w','15',peerip,'18082'],input=request,capture_output=True,text=True,timeout=25)
     if 'Nodo de laboratorio' not in r.stdout:raise AssertionError('El tráfico del teléfono no llega al peer por el túnel: '+r.stderr[:200])
     print('PASS: aplicación Android/shell alcanza otro nodo por la VPN del teléfono',flush=True)
     r=subprocess.run([adb,'-s',serial,'shell','/data/local/tmp/hgw-udp-probe',peerip],capture_output=True,text=True,timeout=15)
     assert 'UDP-Gateway-verificado' in r.stdout, 'UDP no atraviesa el túnel: '+r.stderr[:200]
     print('PASS: UDP del teléfono a otro nodo a través del túnel',flush=True)
     r=subprocess.run([adb,'-s',serial,'shell','/data/local/tmp/hgw-udp-probe','--dns','laboratorio.lab.example.test'],capture_output=True,text=True,timeout=15)
     assert 'MagicDNS-Gateway-verificado' in r.stdout, 'MagicDNS no resuelve el nodo: '+r.stderr[:200]
     print('PASS: consulta MagicDNS directa desde el teléfono por el DNS de la VPN',flush=True)
     r=subprocess.run([adb,'-s',serial,'shell','/data/local/tmp/hgw-udp-probe','laboratorio.lab.example.test'],capture_output=True,text=True,timeout=15)
     if 'UDP-Gateway-verificado' in r.stdout:
      print('PASS: resolvedor DNS del sistema Android',flush=True)
     else:
      print('PENDIENTE: el resolvedor Android no usa MagicDNS; revisa el DNS privado estricto',flush=True)
    with urllib.request.urlopen('http://127.0.0.1:18482/done',timeout=15) as r:assert r.status==200
    checked=True
   if phase=='passed':success=True;break
   if phase=='failed':raise AssertionError(v.get('error'))
 assert success and checked, 'La prueba Android no se completó'
 print('PASS: laboratorio Android completo',flush=True)
finally:
 if peer:peer.terminate();peer.wait(timeout=10)
 subprocess.run([adb,'-s',serial,'shell','rm','-f',remote],capture_output=True)
