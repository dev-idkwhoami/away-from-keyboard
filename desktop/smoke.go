//go:build smoke

package main

import (
	"encoding/json"
	"fmt"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Explicit opt-in: opens real windows and temporarily tests clipboard ownership.
func main() {
	if err := nativeSmoke(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
func nativeSmoke() error {
	root, err := os.MkdirTemp("", "afk-native-test-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(root)
	if data := os.Getenv("AFK_TEST_DATA"); data != "" {
		for _, dir := range []string{"models", "runtime", "nemotron"} {
			if err = os.Symlink(filepath.Join(data, dir), filepath.Join(root, dir)); err != nil {
				return err
			}
		}
	}
	s, err := newStudio(root)
	if err != nil {
		return err
	}
	s.saved.Settings.Theme = "dark"
	s.saved.Settings.Hotkey = ""
	if os.Getenv("AFK_TEST_COMMAND") == "" {
		instanceID += ".smoke"
	}
	app := createApp(s, false)
	var mu sync.Mutex
	results := map[string]string{}
	app.Event.On("afk:test", func(e *application.CustomEvent) {
		b, _ := json.Marshal(e.Data)
		mu.Lock()
		results[string(b)] = "ok"
		mu.Unlock()
	})
	script := `(async()=>{
 const call=async(name,...args)=>{const r=await fetch('/wails/runtime',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({object:0,method:0,args:{'call-id':crypto.randomUUID(),methodName:'main.Studio.'+name,args}})});if(!r.ok)throw Error(await r.text());const text=await r.text();return text?JSON.parse(text):null;};
 const report=async(data)=>fetch('/wails/runtime',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({object:3,method:0,args:{name:'afk:test',data}})});
 try {await new Promise(r=>setTimeout(r,1200));const state=await call('GetState');if(state.models.length!==4)throw Error('catalogue missing');if(!['dictation','meeting'].includes(state.settings.mode))throw Error('capture mode missing');const sources=await call('GetAudioSources');if(!Array.isArray(sources))throw Error('audio source binding failed');if(!location.search.includes('bubble')&&document.querySelectorAll('.studio-modes>section').length!==2)throw Error('meeting layout missing');if(typeof state.diarizationReady!=='boolean'||typeof state.settings.diarization!=='boolean')throw Error('speaker configuration missing');if(document.body.innerText.includes('BROWSER PREVIEW'))throw Error('native detection failed');if(document.querySelector('[role=alert]'))throw Error(document.querySelector('[role=alert]').textContent);await call('SetSettings',{...state.settings,theme:'light'});await new Promise(r=>setTimeout(r,200));if(document.documentElement.classList.contains('dark'))throw Error('theme event missing');await report({ok:true,bubble:location.search.includes('bubble'),url:location.href,text:document.body.innerText.slice(0,90),background:getComputedStyle(document.body).backgroundColor});}catch(e){await report({error:String(e),url:location.href});}
 })();`
	for _, w := range []*application.WebviewWindow{s.main, s.bubble} {
		win := w
		win.OnWindowEvent(events.Linux.WindowLoadFinished, func(*application.WindowEvent) { win.ExecJS(script) })
	}
	check := make(chan error, 1)
	go func() {
		defer app.Quit()
		time.Sleep(4 * time.Second)
		s.mu.Lock()
		s.status = "recording" // Exercise the overlay without opening a microphone.
		s.mu.Unlock()
		s.ShowBubble()
		time.Sleep(time.Second)
		if err := probeBubble(s.bubble, 340, 92); err != nil {
			check <- err
			return
		}
		s.ExpandBubble()
		time.Sleep(time.Second)
		if err := probeBubble(s.bubble, 560, 210); err != nil {
			check <- err
			return
		}
		s.ExpandBubble()
		time.Sleep(time.Second)
		if err := probeBubble(s.bubble, 340, 92); err != nil {
			check <- err
			return
		}
		s.app.Event.Emit("afk:state", s.GetState())
		for position := range bubblePositions {
			v := s.GetState().Settings
			v.BubblePosition = position
			if err := s.SetSettings(v); err != nil {
				check <- err
				return
			}
			time.Sleep(250 * time.Millisecond)
			screen := app.Screen.GetPrimary()
			wantX, wantY := bubbleCoordinates(screen.WorkArea, 340, 92, position)
			x, y := s.bubble.Position()
			if x != wantX || y != wantY {
				check <- fmt.Errorf("%s native position %d,%d; want %d,%d", position, x, y, wantX, wantY)
				return
			}
			fmt.Printf("PLACEMENT PASS: %s = %d,%d\n", position, x, y)
		}
		s.HideBubble()
		s.ShowBubble()
		time.Sleep(250 * time.Millisecond)
		screen := app.Screen.GetPrimary()
		x, y := s.bubble.Position()
		wx, wy := bubbleCoordinates(screen.WorkArea, 340, 92, s.GetState().Settings.BubblePosition)
		if x != wx || y != wy {
			check <- fmt.Errorf("respawn misplaced")
			return
		}
		old, hadText := app.Clipboard.Text()
		defer func() {
			if hadText {
				app.Clipboard.SetText(old)
			}
		}()
		if err := s.CopyText("AFK Grüße clipboard test"); err != nil {
			check <- err
			return
		}
		if got, ok := app.Clipboard.Text(); !ok || got != "AFK Grüße clipboard test" {
			check <- fmt.Errorf("clipboard readback failed: %q", got)
			return
		}
		s.main.Close()
		s.bubble.Close()
		time.Sleep(300 * time.Millisecond)
		if s.main.IsVisible() || s.bubble.IsVisible() || s.GetState().BubbleVisible || s.GetState().Status != "recording" {
			check <- fmt.Errorf("closing windows did not hide them while preserving recording")
			return
		}
		s.OpenStudio()
		time.Sleep(200 * time.Millisecond)
		if !s.main.IsVisible() {
			check <- fmt.Errorf("could not reopen hidden Studio")
			return
		}
		fmt.Println("TRAY LIFECYCLE PASS: close hides both windows, preserves recording, and Studio reopens")
		s.HideBubble()
		s.mu.Lock()
		s.status = "idle"
		s.mu.Unlock()
		if wav := os.Getenv("AFK_TEST_WAV"); wav != "" {
			toggle := s.Toggle
			if command := os.Getenv("AFK_TEST_COMMAND"); command != "" {
				toggle = func() error { return exec.Command(command, "--toggle", "--data", root).Run() }
			}
			if err := toggle(); err != nil {
				check <- err
				return
			}
			deadline := time.Now().Add(12 * time.Second)
			for (s.GetState().Status == "loading" || s.GetState().Status == "idle") && time.Now().Before(deadline) {
				time.Sleep(100 * time.Millisecond)
			}
			if s.GetState().Status != "recording" {
				check <- fmt.Errorf("not recording: %+v", s.GetState())
				return
			}
			time.Sleep(time.Second)
			if err := exec.Command("paplay", "--device="+os.Getenv("AFK_TEST_SINK"), wav).Run(); err != nil {
				check <- err
				return
			}
			time.Sleep(2 * time.Second)
			if err := toggle(); err != nil {
				check <- err
				return
			}
			s.mu.Lock()
			done := s.done
			s.mu.Unlock()
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				check <- fmt.Errorf("stop timed out")
				return
			}
			state := s.GetState()
			got, ok := app.Clipboard.Text()
			if state.Error != "" || !strings.Contains(strings.ToLower(state.Transcript), strings.ToLower(os.Getenv("AFK_TEST_EXPECT"))) || len(state.History) != 1 || !ok || got != state.Transcript {
				check <- fmt.Errorf("dictation failed: %+v clipboard=%q", state, got)
				return
			}
			loaded, err := newStudio(root)
			if err != nil || len(loaded.saved.History) != 1 || loaded.saved.History[0].Text != got {
				check <- fmt.Errorf("persisted history mismatch: %v", err)
				return
			}
			fmt.Printf("DICTATION PASS: %q; clipboard and history match\n", got)
		}
		check <- nil
	}()
	if err = app.Run(); err != nil {
		return err
	}
	if err := <-check; err != nil {
		return err
	}
	mu.Lock()
	defer mu.Unlock()
	if len(results) < 2 {
		return fmt.Errorf("missing window reports: %v", results)
	}
	for result := range results {
		fmt.Println(result)
		if strings.Contains(result, `"error"`) {
			return fmt.Errorf("native check: %s", result)
		}
	}
	if _, err = os.Stat(filepath.Join(root, "desktop-state.json")); err != nil {
		return err
	}
	return nil
}
