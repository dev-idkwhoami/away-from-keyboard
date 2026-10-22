<script lang="ts">
 import {untrack} from 'svelte';
 import {Button} from '$lib/components/ui/button';
 import * as Select from '$lib/components/ui/select';
 import MeetingPanel from './MeetingPanel.svelte';
 import Icon from './Icon.svelte';
 import {studio,act,setting,type MeetingDetail} from './studio.svelte';
 let selected=$state('');
 let detail=$state<MeetingDetail|null>(null);
 let loading=$state(false);
 let saving=$state(false);
 let drafts=$state<Record<string,string>>({});
 let copied=$state(false);
 let confirmProcess=$state(false);
 let request=0;
 let snapshot=$derived(studio.state);
 let busy=$derived(!['idle','error'].includes(snapshot.status)||!!snapshot.preparing);
 let processing=$derived(['meeting-processing','meeting-cancelling'].includes(snapshot.status));
 let current=$derived(processing&&snapshot.meetingFolder.endsWith('/'+selected));
 let model=$derived(snapshot.models.find(m=>m.id===snapshot.settings.model));
 let speakers=$derived([...new Set(detail?.turns.map(t=>t.speaker)||[])]);
 let transcript=$derived(detail?.turns.length ? detail.turns.map(t=>`[${time(t.start)}] ${detail?.speakerNames[t.speaker]||t.speaker}: ${t.text}`).join('\n') : detail?.transcript||'');
 function time(seconds:number){return `${Math.floor(seconds/60)}:${Math.floor(seconds%60).toString().padStart(2,'0')}`;}
 async function load(id:string){
  const ticket=++request;loading=true;
  const value=await act('GetMeeting',id) as MeetingDetail|undefined;
  if(ticket!==request||selected!==id)return;
  detail=value||null;drafts={...(value?.speakerNames||{})};loading=false;
 }
 $effect(()=>{const id=selected;const status=snapshot.status;const ready=studio.ready;
  if(id&&ready)untrack(()=>void load(id));
  return ()=>{request++;};
 });
 function open(id:string){detail=null;selected=id;confirmProcess=false;copied=false;}
 async function rename(speaker:string){saving=true;await act('RenameMeetingSpeaker',selected,speaker,drafts[speaker]||'');if(!studio.notice)await load(selected);saving=false;}
 async function copy(){await act('CopyText',transcript);copied=!studio.notice;}
 function process(){if(detail?.recording.processed&&!confirmProcess){confirmProcess=true;return;}confirmProcess=false;void act('ProcessMeeting',selected);}
</script>
{#if selected}
 <div class="section-summary"><Button variant="ghost" onclick={()=>{selected='';detail=null;}}>← All meetings</Button><span>{current?'PROCESSING RECORDING':'MEETING TRANSCRIPT'}</span></div>
 {#if loading&&!detail}<section class="empty panel"><p>Loading meeting…</p></section>
 {:else if detail}
  <section class="meeting-detail-head panel">
   <div><h2>{new Date(detail.recording.started).toLocaleString()}</h2><p>{time(detail.recording.seconds)} · {detail.recording.processed?'Transcript ready':'Audio saved'}</p></div>
   <div class="meeting-detail-actions">
    {#if transcript}<Button variant="outline" onclick={copy}><Icon name="copy"/>{copied?'Copied':'Copy transcript'}</Button>{/if}
    {#if current}<Button variant="outline" disabled={snapshot.status==='meeting-cancelling'} onclick={()=>act('CancelMeetingProcessing')}>{snapshot.status==='meeting-cancelling'?'Cancelling…':'Cancel processing'}</Button>
    {:else}<Button disabled={busy||saving||!snapshot.diarizationReady||!model?.installed} onclick={process}>{confirmProcess?'Confirm reprocessing':detail.recording.processed?'Process again':'Process recording'}</Button>{/if}
   </div>
  </section>
  <p class="footnote">Processing uses {model?.name||snapshot.settings.model} · {snapshot.settings.language==='auto'?'auto language':snapshot.settings.language.toUpperCase()}.</p>
  {#if !snapshot.diarizationReady||!model?.installed}<p class="footnote">Prepare your selected Whisper model and Nemotron in Models before processing.</p>{/if}
  {#if confirmProcess}<p class="notice">Reprocessing replaces this transcript. Saved names remain attached to speaker labels; check them afterward because speaker assignments can change.</p>{/if}
  {#if current}<p class="notice" role="status">Processing this meeting. You can leave this page; the transcript will be available here when it finishes.</p>{/if}
  {#if speakers.length}
   <section class="meeting-speakers panel"><h3>SPEAKERS</h3><p>Names are saved for this meeting and used when copying its transcript.</p>
    {#each speakers as speaker}
     <form onsubmit={e=>{e.preventDefault();void rename(speaker);}}>
      <label><span>{speaker}</span><input aria-label={`Name for ${speaker}`} placeholder={speaker} maxlength="80" bind:value={drafts[speaker]} disabled={current||saving}/></label>
      <Button type="submit" variant="outline" disabled={current||saving}>{saving?'Saving…':'Save name'}</Button>
     </form>
    {/each}
   </section>
  {/if}
  <section class="meeting-transcript panel" aria-label="Meeting transcript">
   <div class="panel-top"><span>TRANSCRIPT</span><span>{detail.turns.length} TURNS</span></div>
   {#if detail.turns.length}
    {#each detail.turns as turn}<article class="speaker-turn"><div><time>{time(turn.start)}</time><strong>{detail.speakerNames[turn.speaker]||turn.speaker}</strong></div><p>{turn.text}</p></article>{/each}
   {:else}<p class="meeting-empty-text">{detail.transcript||(detail.recording.processed?'No speech was detected.':'Process this recording when you’re ready. Its transcript will appear here.')}</p>{/if}
  </section>
 {:else}<section class="empty panel"><p>The meeting could not be loaded.</p><Button variant="outline" onclick={()=>load(selected)}>Retry</Button></section>{/if}
{:else}
 <div class="meetings-overview">
  <MeetingPanel/>
  <section class="meeting-library panel">
   <div class="panel-top"><span>MEETING HISTORY</span><span class="tag">{snapshot.meetings.length} SAVED</span></div>
   <p>Open a meeting to read its transcript, name speakers, or process its recording.</p>
   <div class="meeting-processing-options">
    <span class="field-label">PROCESSING MODEL</span>
    <Select.Root type="single" value={snapshot.settings.model} disabled={busy} onValueChange={v=>{if(v)setting({model:v});}}><Select.Trigger aria-label="Meeting processing model">{model?.name||snapshot.settings.model}</Select.Trigger><Select.Content>{#each snapshot.models as m}<Select.Item value={m.id} label={m.name}>{m.name}{m.installed?'':' · not downloaded'}</Select.Item>{/each}</Select.Content></Select.Root>
    <Select.Root type="single" value={snapshot.settings.language} disabled={busy} onValueChange={v=>{if(v)setting({language:v});}}><Select.Trigger aria-label="Meeting language">{snapshot.settings.language==='auto'?'Auto language':snapshot.settings.language==='de'?'Deutsch':'English'}</Select.Trigger><Select.Content>{#each [{id:'auto',name:'Auto language'},{id:'en',name:'English'},{id:'de',name:'Deutsch'}] as l}<Select.Item value={l.id} label={l.name}>{l.name}</Select.Item>{/each}</Select.Content></Select.Root>
   </div>
   {#if !snapshot.meetings.length}<div class="meeting-empty-text">No meetings yet. Record one here; processing can wait until later.</div>{/if}
   {#each snapshot.meetings as recording}
    <div class="meeting-recording-row"><div><strong>{new Date(recording.started).toLocaleString()}</strong><span>{time(recording.seconds)} · {processing&&snapshot.meetingFolder.endsWith('/'+recording.id)?'Processing…':recording.processed?'Transcript ready':'Ready to process'}</span></div><Button variant="outline" onclick={()=>open(recording.id)}>Open meeting <Icon name="arrow"/></Button></div>
   {/each}
  </section>
 </div>
{/if}
