package main

import (
	"math"
	"os"
	"path/filepath"
	"testing"
)

func TestSettingsPersistenceAndHistory(t *testing.T) {
	root := t.TempDir()
	s, err := newStudio(root)
	if err != nil {
		t.Fatal(err)
	}
	v := s.GetState().Settings
	v.Theme = "light"
	v.Language = "de"
	if err = s.SetSettings(v); err != nil {
		t.Fatal(err)
	}
	s.saved.History = []Entry{{ID: "one", Text: "Schöne Grüße"}, {ID: "two", Text: "Hello"}}
	if err = s.DeleteHistory("two"); err != nil {
		t.Fatal(err)
	}
	loaded, err := newStudio(root)
	if err != nil {
		t.Fatal(err)
	}
	got := loaded.GetState()
	if got.Settings != v || len(got.History) != 1 || got.History[0].Text != "Schöne Grüße" {
		t.Fatalf("bad persisted state: %+v", got)
	}
	got.History[0].Text = "mutated"
	if loaded.GetState().History[0].Text == "mutated" {
		t.Fatal("state leaked mutable history")
	}
	st, _ := os.Stat(filepath.Join(root, "desktop-state.json"))
	if st.Mode().Perm() != 0600 {
		t.Fatalf("state permissions: %v", st.Mode())
	}
	if err = loaded.ClearHistory(); err != nil {
		t.Fatal(err)
	}
	loaded, err = newStudio(root)
	if err != nil || len(loaded.GetState().History) != 0 {
		t.Fatal("clear not persisted", err)
	}
}
func TestSettingsRejectInvalidAndActiveEngineChanges(t *testing.T) {
	s, _ := newStudio(t.TempDir())
	good := s.saved.Settings
	for _, bad := range []Settings{{Model: "bogus"}, func() Settings { v := good; v.Silence = math.NaN(); return v }(), func() Settings { v := good; v.Language = "fr"; return v }()} {
		if s.SetSettings(bad) == nil {
			t.Fatal("accepted invalid settings")
		}
	}
	s.status = "recording"
	v := good
	v.Model = "turbo-q5"
	if s.SetSettings(v) == nil {
		t.Fatal("changed active engine")
	}
	v = good
	v.Theme = "light"
	if err := s.SetSettings(v); err != nil {
		t.Fatal(err)
	}
}
func TestJoinText(t *testing.T) {
	for _, tc := range [][3]string{{"Hello there friend", "there friend again", "Hello there friend again"}, {"very", "very good", "very very good"}, {"", " Hallo Welt ", "Hallo Welt"}, {"one two", "", "one two"}} {
		if got := joinText(tc[0], tc[1]); got != tc[2] {
			t.Errorf("got %q want %q", got, tc[2])
		}
	}
}
func TestFailedSaveRollsBack(t *testing.T) {
	root := filepath.Join(t.TempDir(), "file")
	os.WriteFile(root, []byte("blocked"), 0600)
	s := &Studio{root: root, saved: savedState{History: []Entry{{ID: "keep"}}}}
	if s.ClearHistory() == nil || len(s.saved.History) != 1 {
		t.Fatal("failed write lost history")
	}
}

func TestBubbleOnlyWhileListening(t *testing.T) {
	s, _ := newStudio(t.TempDir())
	for _, status := range []string{"idle", "loading", "stopping", "error"} {
		s.status = status
		s.ShowBubble()
		if s.GetState().BubbleVisible {
			t.Fatalf("bubble shown while %s", status)
		}
	}
	s.status = "recording"
	s.ShowBubble()
	if !s.GetState().BubbleVisible {
		t.Fatal("bubble hidden while listening")
	}
	if err := s.Toggle(); err != nil {
		t.Fatal(err)
	}
	if s.GetState().BubbleVisible {
		t.Fatal("bubble remained visible after stop")
	}
	s.ShowBubble()
	if s.GetState().BubbleVisible {
		t.Fatal("bubble reopened while stopping")
	}
}
