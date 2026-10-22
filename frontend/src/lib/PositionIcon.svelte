<script lang="ts">
 let {position}:{position:string}=$props();
 const anchors:Record<string,{edge:string;x:number;y:number}>={
  'top-left':{edge:'M4 13V4h9',x:7,y:7},
  'top-center':{edge:'M4 8V4h16v4',x:10,y:7},
  'top-right':{edge:'M11 4h9v9',x:13,y:7},
  'middle-left':{edge:'M8 4H4v16h4',x:7,y:10.5},
  'center':{edge:'',x:10,y:10.5},
  'middle-right':{edge:'M16 4h4v16h-4',x:13,y:10.5},
  'bottom-left':{edge:'M4 11v9h9',x:7,y:14},
  'bottom-center':{edge:'M4 16v4h16v-4',x:10,y:14},
  'bottom-right':{edge:'M11 20h9v-9',x:13,y:14},
 };
 let anchor=$derived(anchors[position]||anchors.center);
</script>
<svg width="24" height="24" viewBox="0 0 24 24" fill="none" aria-hidden="true">
 <rect x="4" y="4" width="16" height="16" rx="1" stroke="currentColor" stroke-width="1" opacity=".18"/>
 {#if anchor.edge}<path d={anchor.edge} stroke="currentColor" stroke-width="2" stroke-linecap="square" stroke-linejoin="miter"/>{/if}
 <rect x={anchor.x} y={anchor.y} width="4" height="3" rx="1" fill="currentColor"/>
</svg>
