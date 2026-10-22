import os,subprocess,time,json,argparse
from pathlib import Path
root=str(Path(__file__).resolve().parent.parent)
os.chdir(root)
parser=argparse.ArgumentParser(description='Native AFK dictation smoke test using an isolated virtual microphone')
parser.add_argument('--wav',required=True)
parser.add_argument('--expect',required=True)
args=parser.parse_args()
wav=str(Path(args.wav).resolve())
if not Path(wav).is_file():parser.error('Speech fixture does not exist')
module=subprocess.check_output(['pactl','load-module','module-null-sink','sink_name=afk_smoke','sink_properties=device.description=AFK_Smoke_Test'],text=True).strip()
try:
 env=os.environ.copy();env.update(SDL_AUDIODRIVER='pulseaudio',SDL_AUDIO_DRIVER='pulseaudio',PULSE_SOURCE='afk_smoke.monitor',AFK_TEST_COMMAND=root+'/bin/afk-desktop',AFK_TEST_SINK='afk_smoke',AFK_TEST_DATA=root+'/.afk',AFK_TEST_WAV=wav,AFK_TEST_EXPECT=args.expect)
 proc=subprocess.Popen([root+'/work/afk-smoke'],env=env)
 moved=set();deadline=time.monotonic()+35
 while proc.poll() is None and time.monotonic()<deadline:
  streams=json.loads(subprocess.check_output(['pactl','-f','json','list','source-outputs'],text=True))
  for stream in streams:
   pid=stream.get('properties',{}).get('application.process.id','')
   if pid and os.path.exists('/proc/'+pid+'/stat') and int(open('/proc/'+pid+'/stat').read().split(') ')[1].split()[1])==proc.pid and stream['index'] not in moved:
    subprocess.run(['pactl','move-source-output',str(stream['index']),'afk_smoke.monitor'],check=True)
    moved.add(stream['index']);print('ROUTED AFK virtual microphone',stream['index'],flush=True)
  time.sleep(.1)
 if proc.poll() is None:proc.kill();proc.wait();raise RuntimeError('smoke timeout')
 if not moved:raise RuntimeError('No isolated source was verified')
 raise SystemExit(proc.returncode)
finally:subprocess.run(['pactl','unload-module',module],check=True)
