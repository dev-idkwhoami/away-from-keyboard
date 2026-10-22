"""Verify app-stream isolation using two synthetic tones on one virtual sink."""
import json, math, os, struct, subprocess, tempfile, time, wave
from pathlib import Path
module=subprocess.check_output(['pactl','load-module','module-null-sink','sink_name=afk_stream_test'],text=True).strip()
processes=[]
try:
 with tempfile.TemporaryDirectory(prefix='afk-app-stream-') as directory:
  for name,hz in [('target',440),('other',880)]:
   path=Path(directory)/(name+'.wav')
   with wave.open(str(path),'wb') as f:
    f.setparams((1,2,16000,0,'NONE','not compressed'))
    f.writeframes(b''.join(struct.pack('<h',int(8000*math.sin(2*math.pi*hz*i/16000))) for i in range(16000*8)))
   processes.append(subprocess.Popen(['paplay','--device=afk_stream_test','--client-name=AFK_Test_'+name,str(path)]))
  index=None
  for _ in range(30):
   streams=json.loads(subprocess.check_output(['pactl','-f','json','list','sink-inputs']))
   for stream in streams:
    if stream['properties'].get('application.name')=='AFK_Test_target':index=stream['index']
   if index is not None:break
   time.sleep(.1)
  assert index is not None,'Target stream did not appear'
  recorder=subprocess.Popen(['parec','--raw','--format=s16le','--rate=16000','--channels=1','--device=afk_stream_test.monitor','--monitor-stream='+str(index)],stdout=subprocess.PIPE)
  processes.append(recorder)
  raw=recorder.stdout.read(32000*2)
  recorder.terminate();recorder.wait()
  samples=struct.unpack('<'+'h'*(len(raw)//2),raw)
  def amplitude(hz):
   re=sum(v*math.cos(2*math.pi*hz*i/16000) for i,v in enumerate(samples))
   im=sum(v*math.sin(2*math.pi*hz*i/16000) for i,v in enumerate(samples))
   return math.hypot(re,im)/len(samples)
  target,other=amplitude(440),amplitude(880)
  assert target>1000 and other<target*.05,(target,other)
  print('PASS app-stream isolation: target amplitude %.1f, unrelated app %.1f'%(target,other))
finally:
 for process in processes:
  if process.poll() is None:process.terminate()
 for process in processes:process.wait()
 subprocess.run(['pactl','unload-module',module],check=True)
