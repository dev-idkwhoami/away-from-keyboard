package main

import (
	"embed"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
	"io/fs"
	"log"
	"os"
	"slices"
)

//go:embed all:assets
var assets embed.FS

var instanceID = "dev.idkwhoami.afk"

func createApp(s *Studio, toggle bool) *application.App {
	// Normal Wayland toplevels cannot be placed at absolute coordinates.
	// Use XWayland when available; this affects AFK only, not the session.
	if os.Getenv("DISPLAY") != "" {
		_ = os.Setenv("GDK_BACKEND", "x11")
	} else {
		s.problem = "Automatic bubble placement requires X11/XWayland (DISPLAY is unavailable)."
	}
	files, _ := fs.Sub(assets, "assets")
	icon, _ := assets.ReadFile("assets/afk-neon.png")
	app := application.New(application.Options{Name: "AFK", Icon: icon, Description: "Away From Keyboard — local voice, no strings attached", Linux: application.LinuxOptions{ProgramName: "afk", ApplicationID: instanceID}, Services: []application.Service{application.NewService(s)}, Assets: application.AssetOptions{Handler: application.AssetFileServerFS(files)}, SingleInstance: &application.SingleInstanceOptions{UniqueID: instanceID, OnSecondInstanceLaunch: func(data application.SecondInstanceData) {
		if slices.Contains(data.Args, "--toggle") {
			if err := s.Toggle(); err != nil {
				log.Print(err)
			}
		} else {
			s.OpenStudio()
		}
	}}})
	s.app = app
	s.main = app.Window.NewWithOptions(application.WebviewWindowOptions{Name: "studio", Title: "AFK — Away From Keyboard", Width: 1160, Height: 800, MinWidth: 900, MinHeight: 680, URL: "/", BackgroundColour: application.NewRGB(11, 12, 16)})
	s.bubble = app.Window.NewWithOptions(application.WebviewWindowOptions{Name: "bubble", Title: "AFK Bubble", Width: 340, Height: 92, URL: "/?bubble", Hidden: true, Frameless: true, DisableResize: true, AlwaysOnTop: true, BackgroundType: application.BackgroundTypeTransparent, BackgroundColour: application.NewRGBA(0, 0, 0, 0)})
	s.main.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) { e.Cancel(); s.main.Hide() })
	s.bubble.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) { e.Cancel(); s.HideBubble() })
	tray := app.SystemTray.New()
	tray.SetIcon(icon)
	tray.SetLabel("AFK")
	tray.SetTooltip("AFK — Away From Keyboard")
	tray.OnClick(s.OpenStudio)

	menu := app.NewMenu()
	menu.Add("Open").OnClick(func(*application.Context) { s.OpenStudio() })
	s.trayFile = menu.Add("Transcribe file").OnClick(func(*application.Context) { go s.trayAction(s.ChooseFile) })
	// KDE sizes native tray menus; non-breaking spaces reserve room before the arrow.
	models := menu.AddSubmenu("Choose Model\u00a0\u00a0\u00a0")
	s.trayModels = make(map[string]*application.MenuItem)
	for _, model := range s.GetState().Models {
		id := model.ID
		item := models.AddRadio(model.Name, false)
		item.OnClick(func(*application.Context) { go s.trayAction(func() error { return s.SelectModel(id) }) })
		s.trayModels[id] = item
	}
	menu.Add("Close").OnClick(func(*application.Context) { s.Quit() })
	s.updateTray(s.GetState())

	tray.SetMenu(menu)

	app.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(_ *application.ApplicationEvent) {
		go s.startHotkey()
		if toggle {
			go func() {
				if err := s.Toggle(); err != nil {
					log.Print(err)
				}
			}()
		}
	})
	return app
}
