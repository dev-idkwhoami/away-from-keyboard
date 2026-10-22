package main

import (
	"github.com/wailsapp/wails/v3/pkg/application"
	"os"
	"path/filepath"
	"testing"
)

func TestBubbleCoordinates(t *testing.T) {
	area := application.Rect{X: -1920, Y: 40, Width: 1920, Height: 1040}
	xs := []int{-1900, -1130, -360}
	ys := []int{60, 514, 968}
	for position, anchor := range bubblePositions {
		x, y := bubbleCoordinates(area, 340, 92, position)
		if x != xs[anchor[0]] || y != ys[anchor[1]] {
			t.Errorf("%s: %d,%d", position, x, y)
		}
	}
	x, y := bubbleCoordinates(area, 560, 210, "bottom-right")
	if x != -580 || y != 850 {
		t.Fatalf("expanded anchor: %d,%d", x, y)
	}
	x, y = bubbleCoordinates(application.Rect{X: 50, Y: 60, Width: 200, Height: 80}, 340, 92, "bottom-right")
	if x != 50 || y != 60 {
		t.Fatalf("oversize clamping: %d,%d", x, y)
	}
}
func TestPlacementMigrationAndPersistence(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "desktop-state.json"), []byte(`{"settings":{"model":"small-q5","language":"de","theme":"dark","bubbleMode":"wave","saveHistory":true,"silence":0.003},"history":[]}`), 0600)
	s, err := newStudio(root)
	if err != nil {
		t.Fatal(err)
	}
	if s.saved.Settings.BubblePosition != "top-center" {
		t.Fatal("missing default for old settings")
	}
	v := s.saved.Settings
	v.BubblePosition = "bottom-right"
	if err = s.SetSettings(v); err != nil {
		t.Fatal(err)
	}
	loaded, err := newStudio(root)
	if err != nil || loaded.saved.Settings.BubblePosition != "bottom-right" {
		t.Fatal("placement not persisted", err)
	}
	v.BubblePosition = "invalid"
	if s.SetSettings(v) == nil {
		t.Fatal("invalid placement accepted")
	}
}
