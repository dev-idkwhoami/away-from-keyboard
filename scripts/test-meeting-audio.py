"""Exercise the record-now/process-later meeting pipeline using two isolated virtual devices."""
import os, subprocess, sys
from pathlib import Path
root=Path(__file__).resolve().parent.parent
modules=[]
def load(*args):
    value=subprocess.check_output(['pactl','load-module',*args],text=True).strip()
    modules.append(value)
try:
    load('module-null-sink','sink_name=afk_test_mic_sink','sink_properties=device.description=AFK_Test_Mic')
    load('module-null-sink','sink_name=afk_test_remote_sink','sink_properties=device.description=AFK_Test_Remote')
    load('module-remap-source','master=afk_test_mic_sink.monitor','source_name=afk_test_microphone','source_properties=device.description=AFK_Test_Microphone device.class=sound')
    env=os.environ.copy()
    env.update(AFK_MEETING_TEST_DATA=str(root/'.afk'),AFK_MEETING_TEST_MIC='afk_test_microphone',AFK_MEETING_TEST_REMOTE='afk_test_remote_sink.monitor',AFK_MEETING_TEST_MIC_SINK='afk_test_mic_sink',AFK_MEETING_TEST_REMOTE_SINK='afk_test_remote_sink',AFK_MEETING_TEST_MIC_WAV=str(root/'work/german.wav'),AFK_MEETING_TEST_REMOTE_WAV=str(root/'work/nemo-source/test_files/diar/ami_en2002d_2132.wav'),GOCACHE=str(root/'work/go-cache'))
    subprocess.run(['go','-C',str(root/'desktop'),'test','-race','-tags','gtk3','-buildvcs=false','-run',('^TestMeetingLifecycleIntegration$' if '--lifecycle-only' in sys.argv else '^Test(LiveMeetingIntegration|MeetingLifecycleIntegration)$'),'-v','-count=1','-timeout=150s'],env=env,check=True)
finally:
    for module in reversed(modules):
        subprocess.run(['pactl','unload-module',module],check=True)
