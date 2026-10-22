<script lang="ts">
 import {Button} from '$lib/components/ui/button';
 import {studio,act} from './studio.svelte';
 let snapshot=$derived(studio.state);
 let busy=$derived(!['idle','error'].includes(snapshot.status)||!!snapshot.preparing);
 let processing=$derived(['meeting-processing','meeting-cancelling'].includes(snapshot.status));
 let model=$derived(snapshot.models.find(m=>m.id===snapshot.settings.model));
</script>
{#if snapshot.meetings.length}
 <section class="meeting-recordings panel">
  <div class="panel-top"><span>RECORDED MEETINGS</span><span class="tag">PROCESS ON DEMAND</span></div>
  <p>Processing uses {model?.name||snapshot.settings.model} · {snapshot.settings.language==='auto'?'auto language':snapshot.settings.language.toUpperCase()}. Your mic becomes “You”; Nemotron labels remote speakers.</p>
  {#if !snapshot.diarizationReady||!model?.installed}<p>Prepare your selected Whisper model and Nemotron in Model Garage before processing. Recording works without them.</p>{/if}
  {#each snapshot.meetings as recording}
   {@const current=processing&&snapshot.meetingFolder.endsWith('/'+recording.id)}
   <div class="meeting-recording-row">
    <div><strong>{new Date(recording.started).toLocaleString()}</strong><span>{Math.floor(recording.seconds/60)}m {recording.seconds%60}s · {recording.processed?'Processed':'Audio saved'}</span></div>
    {#if current}<Button variant="outline" disabled={snapshot.status==='meeting-cancelling'} onclick={()=>act('CancelMeetingProcessing')}>{snapshot.status==='meeting-cancelling'?'Cancelling…':'Cancel processing'}</Button>
    {:else}<Button variant="outline" disabled={busy||!snapshot.diarizationReady||!model?.installed} onclick={()=>act('ProcessMeeting',recording.id)}>{recording.processed?'Process again':'Process recording'}</Button>{/if}
   </div>
  {/each}
 </section>
{/if}
