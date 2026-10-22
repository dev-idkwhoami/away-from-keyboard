<script lang="ts">
 import {onMount} from 'svelte';
 import {studio} from './studio.svelte';
 let {active=false,large=false}:{active?:boolean;large?:boolean}=$props();
 let bars=$state<number[]>(Array(23).fill(0));
 let envelope=0;
 onMount(()=>{
  const timer=setInterval(()=>{
   const live=active&&['recording','meeting'].includes(studio.state.status);
   const target=live&&Date.now()-studio.audioLevelAt<300?studio.audioLevel:0;
   envelope=target>envelope?target:envelope*.65;
   if(envelope<.005)envelope=0;
   bars=[...bars.slice(1),envelope];
  },50);
  return ()=>clearInterval(timer);
 });
</script>
<div class:active class:large class="wave" aria-hidden="true">{#each bars as level}<i style={`--bar:${4+96*level}%`}></i>{/each}</div>
