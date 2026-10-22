export type Settings = {mode:string;meetingMic:string;meetingAudio:string;diarization:boolean;hotkey:string;model:string;language:string;theme:string;bubbleMode:string;bubblePosition:string;saveHistory:boolean;silence:number};
export type Model = {id:string;name:string;bytes:number;autoLanguage:boolean;installed:boolean};
export type Entry = {meetingFolder?:string;id:string;text:string;model:string;language:string;created:string;seconds:number};
export type SpeakerTurn={start:number;end:number;speaker:string;text:string};
export type MeetingRecording={id:string;started:string;seconds:number;processed:boolean};
export type State = {meetings:MeetingRecording[];meetingFolder:string;diarizationReady:boolean;turns:SpeakerTurn[];fileName:string;fileStage:string;hotkeyActive:boolean;hotkeyError:string;settings:Settings;models:Model[];history:Entry[];status:string;transcript:string;error:string;preparing:string;started:number;bubbleVisible:boolean};
export const studio = $state<{state:State;native:boolean;ready:boolean;notice:string}>({native:false,ready:false,notice:'',state:{meetings:[],meetingFolder:"",diarizationReady:false,turns:[],fileName:'',fileStage:'',hotkeyActive:false,hotkeyError:"",settings:{mode:"dictation",meetingMic:"@DEFAULT_SOURCE@",meetingAudio:"@DEFAULT_MONITOR@",diarization:false,hotkey:"Ctrl+Shift+Space",model:'small-q5',language:'auto',theme:'system',bubbleMode:'wave',bubblePosition:'top-center',saveHistory:true,silence:.003},models:[],history:[],status:'idle',transcript:'',error:'',preparing:'',started:0,bubbleVisible:false}});
let runtime: typeof import('@wailsio/runtime') | undefined;
export async function connect() {
  studio.native = location.protocol === 'wails:' || location.hostname === 'wails.localhost';
  if (studio.native) {
    runtime = await import('@wailsio/runtime');
    const off = runtime.Events.On('afk:state', e => {studio.state = e.data as State;});
    try {studio.state = await runtime.Call.ByName('main.Studio.GetState') as State;} catch(e) {studio.notice = String(e);}
    studio.ready = true;
    return off;
  }
  studio.state.models = [{id:'small-q5',name:'Small Q5',bytes:190085487,autoLanguage:true,installed:false},{id:'turbo-q5',name:'Turbo Q5',bytes:574041195,autoLanguage:true,installed:false},{id:'turbo-q8',name:'Turbo Q8',bytes:874188075,autoLanguage:true,installed:false},{id:'large-v3-q5',name:'Large v3 Q5',bytes:1081140203,autoLanguage:true,installed:false}];
  studio.ready=true;
  return ()=>{};
}
export async function act(method:string,...args:unknown[]) {
  studio.notice='';
  try {
    if(runtime) return await runtime.Call.ByName(`main.Studio.${method}`,...args);
    if(method==='SetHotkey'){studio.state.settings.hotkey=args[0] as string;studio.notice='Shortcut saved for this preview. Global registration runs in the desktop app.';}
    else if(method==='SetSettings') studio.state.settings=args[0] as Settings;
    else if(method==='ExpandBubble') studio.state.settings.bubbleMode=studio.state.settings.bubbleMode==='wave'?'text':'wave';
    else studio.notice='Open the desktop app to use this feature.';
  } catch(e) {studio.notice=String(e);}
}
export function setting(patch:Partial<Settings>) {return act('SetSettings',{...studio.state.settings,...patch});}
export function size(bytes:number) {return bytes>=1e9?`${(bytes/1e9).toFixed(2)} GB`:`${Math.round(bytes/1e6)} MB`;}
