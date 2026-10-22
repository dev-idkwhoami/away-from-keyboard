<script lang="ts">
 import {Button} from '$lib/components/ui/button';
 import * as Select from '$lib/components/ui/select';
 import {studio,act} from '$lib/studio.svelte';
 import Icon from '$lib/Icon.svelte';
 const modifiers=['Ctrl','Alt','Shift','Super'];
 const keys=['Space',...'ABCDEFGHIJKLMNOPQRSTUVWXYZ',...'0123456789',...Array.from({length:24},(_,i)=>'F'+(i+1))];
 let selected=$state<string[]>(['Ctrl','Shift']);
 let key=$state('Space');
 let busy=$state(false);
 $effect(()=>{
   const parts=(studio.state.settings.hotkey||'Ctrl+Shift+Space').split('+');
   key=parts.pop()!;selected=parts;
 });
 const draft=$derived([...modifiers.filter(m=>selected.includes(m)),key].join('+'));
 const valid=$derived(/^F\d+$/.test(key)||selected.some(m=>m==='Ctrl'||m==='Alt'||m==='Super'));
 async function apply(value:string){busy=true;try{await act('SetHotkey',value);}finally{busy=false;}}
</script>
<section class="shortcut-card panel">
 <Icon name="bolt" size={32}/>
 <div class="hotkey-content">
  <h2>ONE SHORTCUT. ANYWHERE.</h2>
  <p>Toggle dictation from any app while AFK is running. Press again to stop and copy.</p>
  <div class="hotkey-controls">
   <div class="theme-options" role="group" aria-label="Shortcut modifiers">
    {#each modifiers as modifier}
     <Button variant={selected.includes(modifier)?'default':'outline'} aria-pressed={selected.includes(modifier)} disabled={busy} onclick={()=>selected=selected.includes(modifier)?selected.filter(m=>m!==modifier):[...selected,modifier]}>{modifier}</Button>
    {/each}
   </div>
   <Select.Root type="single" bind:value={key} disabled={busy}>
    <Select.Trigger aria-label="Shortcut key">{key}</Select.Trigger>
    <Select.Content>{#each keys as value}<Select.Item {value} label={value}>{value}</Select.Item>{/each}</Select.Content>
   </Select.Root>
   <Button disabled={busy||!valid} onclick={()=>apply(draft)}>{busy?'APPLYING…':'APPLY SHORTCUT'}</Button>
   <Button variant="outline" disabled={busy||!studio.state.settings.hotkey} onclick={()=>apply('')}>DISABLE</Button>
  </div>
  <p class="footnote" role="status">{#if studio.state.hotkeyError}{studio.state.hotkeyError}{:else if studio.state.hotkeyActive}Global shortcut active: {studio.state.settings.hotkey.split('+').join(' + ')}{:else if !studio.state.settings.hotkey}Global shortcut disabled.{:else if !studio.native}Preview only — global registration requires the desktop app.{:else}Global shortcut is not active. Apply to retry.{/if}</p>
  {#if !valid}<p class="footnote">Add Ctrl, Alt, or Super for letters, numbers, and Space.</p>{/if}
 </div>
</section>
