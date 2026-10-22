<script lang="ts">
 import {Button} from '$lib/components/ui/button';
 import Icon from './Icon.svelte';import Wave from './Wave.svelte';import {studio,act} from './studio.svelte';
 let {preview=false}:{preview?:boolean}=$props();
 let active=$derived(['loading','recording','stopping','meeting-loading','meeting','meeting-stopping'].includes(studio.state.status));
 let expanded=$derived(studio.state.settings.bubbleMode==='text');
 let tail=$state<HTMLDivElement>();
 $effect(()=>{studio.state.transcript;if(tail)tail.scrollTop=tail.scrollHeight;});
</script>
<div class="voice-bubble" class:expanded class:preview>
 <div class="bubble-row">
  <Button variant="ghost" class="bubble-mark" onclick={()=>act('OpenStudio')} aria-label="Open AFK"><img src="/afk-neon.png" alt="" width="32" height="32"/></Button>
  <Button variant="ghost" class="bubble-center" onclick={()=>act('ExpandBubble')} aria-label={expanded?'Collapse transcript':'Expand transcript'}><Wave {active}/><span class="bubble-status">{['recording','meeting'].includes(studio.state.status)?'LISTENING':studio.state.status.endsWith('loading')?'LOADING':studio.state.status.endsWith('stopping')?'FINISHING':'READY'}</span></Button>
  <Button class="bubble-record" disabled={!studio.ready||studio.state.status.endsWith('stopping')} onclick={()=>act('Toggle')} aria-label={active?'Stop recording':'Start recording'}><Icon name={active?'stop':'mic'}/></Button>
  <Button variant="ghost" class="bubble-close" onclick={()=>act('HideBubble')} aria-label="Hide bubble to system tray" title="Hide to tray — recording continues"><Icon name="close"/></Button>
 </div>
 {#if expanded}<div class="bubble-transcript" bind:this={tail}>{studio.state.transcript||(studio.state.status==='meeting'?'Recording meeting audio. Process it later in Studio.':'Go on. We’re listening.')}</div><div class="bubble-footer"><span>ON DEVICE · {studio.state.settings.language.toUpperCase()}</span><Button variant="ghost" onclick={()=>act('Quit')} aria-label="Quit AFK"><Icon name="power"/></Button></div>{/if}
</div>
