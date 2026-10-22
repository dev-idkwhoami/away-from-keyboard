<script lang="ts">let {active=false,large=false}:{active?:boolean;large?:boolean}=$props();</script>
<div class:active class:large class="wave" aria-hidden="true">{#each [12,23,38,19,44,62,34,52,76,45,66,36,56,81,48,30,60,40,24,48,27,15,25] as h,i}<i style={`--bar:${h}%;--delay:-${i*.137}s;--speed:${.65+(i%5)*.13}s`}></i>{/each}</div>
