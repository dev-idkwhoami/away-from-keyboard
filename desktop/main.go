//go:build !smoke

package main

import (
	"afk/internal/engine"
	"flag"
	"log"
	"os"
	"path/filepath"
)

func main() {
	exe, err := os.Executable()
	if err != nil {
		log.Fatal(err)
	}
	data := flag.String("data", engine.DefaultDataDir(filepath.Join(filepath.Dir(filepath.Dir(exe)), ".afk")), "AFK model and desktop state directory")
	toggle := flag.Bool("toggle", false, "toggle dictation in the running AFK instance (for a KDE shortcut)")
	flag.Parse()
	root, err := filepath.Abs(*data)
	if err != nil {
		log.Fatal(err)
	}
	s, err := newStudio(root)
	if err != nil {
		log.Fatal(err)
	}
	if err := createApp(s, *toggle).Run(); err != nil {
		log.Fatal(err)
	}
}
