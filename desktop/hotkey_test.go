package main

import (
	"errors"
	"github.com/godbus/dbus/v5"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestHotkeyParsing(t *testing.T) {
	for _, tc := range []struct {
		input, want string
		code        int32
	}{
		{"", "", 0}, {"Shift+Ctrl+Space", "Ctrl+Shift+Space", 0x06000020},
		{"Super+Alt+K", "Alt+Super+K", 0x1800004b}, {"F24", "F24", 0x01000047},
	} {
		got, code, err := parseHotkey(tc.input)
		if err != nil || got != tc.want || code != tc.code {
			t.Fatalf("%q: %q %x %v", tc.input, got, code, err)
		}
	}
	for _, bad := range []string{"A", "Shift+A", "Space", "Ctrl+Ctrl+A", "Ctrl+", "Ctrl+a", "F25", "Meta+A"} {
		if _, _, err := parseHotkey(bad); err == nil {
			t.Errorf("accepted %q", bad)
		}
	}
}

type fakeShortcut struct {
	value  string
	reject string
}

func (f *fakeShortcut) Apply(v string) error {
	if v == f.reject {
		return errors.New("conflict")
	}
	f.value = v
	return nil
}
func (f *fakeShortcut) Close() error { return nil }
func TestHotkeyPersistenceAndRollback(t *testing.T) {
	s, _ := newStudio(t.TempDir())
	f := &fakeShortcut{reject: "Ctrl+K"}
	s.shortcut = f
	if err := s.SetHotkey("Alt+F8"); err != nil {
		t.Fatal(err)
	}
	loaded, _ := newStudio(s.root)
	if loaded.saved.Settings.Hotkey != "Alt+F8" || !s.GetState().HotkeyActive {
		t.Fatal("not persisted/active")
	}
	if s.SetHotkey("Ctrl+K") == nil || s.saved.Settings.Hotkey != "Alt+F8" || f.value != "Alt+F8" {
		t.Fatal("conflict lost previous shortcut")
	}
	blocked := filepath.Join(t.TempDir(), "file")
	os.WriteFile(blocked, []byte("blocked"), 0600)
	s.root = blocked
	if s.SetHotkey("Ctrl+J") == nil || s.saved.Settings.Hotkey != "Alt+F8" || f.value != "Alt+F8" {
		t.Fatal("save failure did not restore shortcut")
	}
}
func TestKDEGlobalShortcutIntegration(t *testing.T) {
	if os.Getenv("AFK_HOTKEY_TEST") != "1" {
		t.Skip("requires desktop session D-Bus")
	}
	activated := make(chan struct{}, 4)
	k, err := newKDEShortcut("dev.idkwhoami.afk.hotkeytest", func() { activated <- struct{}{} }, func(bool, error) {})
	if err != nil {
		t.Fatal(err)
	}
	defer k.Close()
	if err = k.Apply("Ctrl+Alt+Shift+F24"); err != nil {
		t.Fatal(err)
	}
	var path dbus.ObjectPath
	if err = k.call("getComponent", k.action[0]).Store(&path); err != nil {
		t.Fatal(err)
	}
	if err = k.conn.Object(accelService, path).Call(componentInterface+".invokeShortcut", 0, hotkeyAction).Err; err != nil {
		t.Fatal(err)
	}
	select {
	case <-activated:
	case <-time.After(3 * time.Second):
		t.Fatal("global activation signal not delivered")
	}
	other, err := newKDEShortcut("dev.idkwhoami.afk.hotkeyconflict", func() {}, func(bool, error) {})
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	if other.Apply("Ctrl+Alt+Shift+F24") == nil {
		t.Fatal("accepted conflicting shortcut")
	}
	if err = k.Apply("Ctrl+Alt+Shift+F23"); err != nil {
		t.Fatal(err)
	}
	_, old, _ := parseHotkey("Ctrl+Alt+Shift+F24")
	var free bool
	if err = k.call("isGlobalShortcutAvailable", old, "").Store(&free); err != nil || !free {
		t.Fatal("old shortcut not released", err)
	}
	if err = k.Apply(""); err != nil {
		t.Fatal(err)
	}
	_, last, _ := parseHotkey("Ctrl+Alt+Shift+F23")
	if err = k.call("isGlobalShortcutAvailable", last, "").Store(&free); err != nil || !free {
		t.Fatal("disable did not release shortcut", err)
	}
	t.Log("KDE registration, activation dispatch, conflict rejection, rebinding and disabling passed")
}
