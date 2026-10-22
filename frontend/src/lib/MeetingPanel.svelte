<script lang="ts">
 import {untrack} from 'svelte';
 import {Button} from '$lib/components/ui/button';
 import * as Select from '$lib/components/ui/select';
 import Icon from './Icon.svelte';
 import {studio,act,setting} from './studio.svelte';
 type Source={id:string;label:string;kind:string};
 let sources=$state<Source[]>([{id:"@DEFAULT_SOURCE@",label:"System default microphone",kind:"mic"},{id:"@DEFAULT_MONITOR@",label:"System default output",kind:"output"}]);let refreshing=$state(false);
 let snapshot=$derived(studio.state);let settings=$derived(snapshot.settings);
 let running=$derived(['meeting-loading','meeting','meeting-stopping'].includes(snapshot.status));
 let busy=$derived(!['idle','error'].includes(snapshot.status));
 async function refresh(){refreshing=true;try{const result=await act('GetAudioSources');if(Array.isArray(result))sources=result;}finally{refreshing=false;}}
 let loaded=$state(false);
 $effect(()=>{if(studio.native&&studio.ready&&!loaded){loaded=true;untrack(()=>void refresh());}});
</script>
<section class="meeting-panel panel" class:mode-selected={settings.mode==='meeting'}>
 <div class="panel-top"><span><span class="status-dot" class:recording={snapshot.status==='meeting'}></span>MEETING RECORDER</span><span class="tag">TWO INPUTS</span></div>
 <div class="meeting-body">
  <h2>MEETING.</h2>
  <p>Record your mic and meeting audio. Transcribe and identify speakers later.</p>
  {#each [{key:'meetingMic',label:'YOUR MICROPHONE',kind:'mic'},{key:'meetingAudio',label:'MEETING AUDIO',kind:'remote'}] as field}
   {@const value=field.key==='meetingMic'?settings.meetingMic:settings.meetingAudio}
   <div class="meeting-field"><span class="field-label">{field.label}</span>
    <Select.Root type="single" {value} disabled={busy} onValueChange={v=>{if(v)setting({[field.key]:v});}}>
     <Select.Trigger aria-label={field.label}>{sources.find(s=>s.id===value)?.label||(value==='@DEFAULT_SOURCE@'?'System default microphone':value==='@DEFAULT_MONITOR@'?'System default output':value?'Saved source · refresh to check':'Choose a source')}</Select.Trigger>
     <Select.Content>{#each sources.filter(s=>field.kind==='mic'?s.kind==='mic':s.kind!=='mic') as source}<Select.Item value={source.id} label={source.label}>{source.kind==='app'?'App · ':source.kind==='output'?'Output · ':''}{source.label}</Select.Item>{/each}</Select.Content>
    </Select.Root>
   </div>
  {/each}
  <Button variant="ghost" class="refresh-audio" disabled={busy||refreshing||!studio.native} onclick={refresh}>{refreshing?'Refreshing…':'Refresh audio sources'}</Button>
  <p class="meeting-hint">Defaults are selected when recording starts. You can also choose a specific device or active app stream. Start playback in Teams, then refresh. Use headphones to reduce echo.</p>
  <Button class="meeting-start" disabled={!studio.ready||!!snapshot.preparing||(busy&&!running)||snapshot.status==='meeting-stopping'||(!running&&(!settings.meetingMic||!settings.meetingAudio))} onclick={()=>act(running?'StopMeeting':'StartMeeting')}><Icon name={running?'stop':'mic'}/>{snapshot.status==='meeting-stopping'?'SAVING AUDIO…':snapshot.status==='meeting-loading'?'CANCEL STARTUP':running?'STOP & SAVE':'RECORD MEETING'}</Button>
  <Button variant="ghost" class="mode-choice" disabled={busy} aria-pressed={settings.mode==='meeting'} onclick={()=>setting({mode:'meeting'})}>{settings.mode==='meeting'?'✓ Global shortcut starts meetings':'Use global shortcut for meetings'}</Button>
  <p class="meeting-hint">Audio only while recording. No transcription models run until you open a saved meeting and choose Process recording.</p>
 </div>
</section>
