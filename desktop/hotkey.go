package main

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"
)

const accelService = "org.kde.kglobalaccel"
const accelInterface = "org.kde.KGlobalAccel"
const componentInterface = "org.kde.kglobalaccel.Component"
const hotkeyAction = "_k_session:toggle-dictation"

// Qt key/modifier values used by KGlobalAccel's public D-Bus interface.
func parseHotkey(value string) (string, int32, error) {
	if value == "" {
		return "", 0, nil
	}
	parts := strings.Split(value, "+")
	modifiers := map[string]int32{"Ctrl": 0x04000000, "Alt": 0x08000000, "Shift": 0x02000000, "Super": 0x10000000}
	var mask int32
	for _, part := range parts[:len(parts)-1] {
		bit, ok := modifiers[part]
		if !ok || mask&bit != 0 {
			return "", 0, errors.New("invalid or duplicate shortcut modifier")
		}
		mask |= bit
	}
	key := parts[len(parts)-1]
	var code int32
	switch {
	case key == "Space":
		code = 0x20
	case len(key) == 1 && ((key[0] >= 'A' && key[0] <= 'Z') || (key[0] >= '0' && key[0] <= '9')):
		code = int32(key[0])
	case strings.HasPrefix(key, "F"):
		n, err := strconv.Atoi(key[1:])
		if err != nil || n < 1 || n > 24 {
			return "", 0, errors.New("choose F1–F24")
		}
		code = int32(0x01000030 + n - 1)
		key = fmt.Sprintf("F%d", n)
	default:
		return "", 0, errors.New("choose a letter, digit, Space, or F1–F24")
	}
	if code < 0x01000030 && mask&(0x04000000|0x08000000|0x10000000) == 0 {
		return "", 0, errors.New("use Ctrl, Alt, or Super with this key")
	}
	canonical := []string{}
	for _, name := range []string{"Ctrl", "Alt", "Shift", "Super"} {
		if mask&modifiers[name] != 0 {
			canonical = append(canonical, name)
		}
	}
	canonical = append(canonical, key)
	return strings.Join(canonical, "+"), mask | code, nil
}

type shortcutBinding interface {
	Apply(string) error
	Close() error
}
type kdeShortcut struct {
	mu       sync.Mutex
	conn     *dbus.Conn
	action   []string
	key      int32
	value    string
	closed   bool
	signals  chan *dbus.Signal
	activate func()
	status   func(bool, error)
}

func newKDEShortcut(component string, activate func(), status func(bool, error)) (*kdeShortcut, error) {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return nil, fmt.Errorf("connect to KDE shortcuts: %w", err)
	}
	k := &kdeShortcut{conn: conn, action: []string{component, hotkeyAction, "AFK — Away From Keyboard", "Toggle dictation"}, signals: make(chan *dbus.Signal, 16), activate: activate, status: status}
	conn.Signal(k.signals)
	if err = conn.AddMatchSignal(dbus.WithMatchSender(accelService), dbus.WithMatchInterface(componentInterface), dbus.WithMatchMember("globalShortcutPressed")); err != nil {
		conn.Close()
		return nil, err
	}
	if err = conn.AddMatchSignal(dbus.WithMatchSender("org.freedesktop.DBus"), dbus.WithMatchInterface("org.freedesktop.DBus"), dbus.WithMatchMember("NameOwnerChanged"), dbus.WithMatchArg(0, accelService)); err != nil {
		conn.Close()
		return nil, err
	}
	go k.listen()
	return k, nil
}
func (k *kdeShortcut) call(method string, args ...interface{}) *dbus.Call {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return k.conn.Object(accelService, dbus.ObjectPath("/kglobalaccel")).CallWithContext(ctx, accelInterface+"."+method, 0, args...)
}
func (k *kdeShortcut) set(key int32) error {
	keys := []int32{}
	if key != 0 {
		keys = append(keys, key)
	}
	var actual []int32
	// SetPresent=2, NoAutoloading=4. Never use the foreign/steal APIs.
	if err := k.call("setShortcut", k.action, keys, uint32(6)).Store(&actual); err != nil {
		return err
	}
	if (key == 0 && len(actual) != 0) || (key != 0 && (len(actual) != 1 || actual[0] != key)) {
		return errors.New("KDE could not assign that shortcut; it may already be in use")
	}
	return nil
}
func (k *kdeShortcut) Apply(value string) error {
	canonical, key, err := parseHotkey(value)
	if err != nil {
		return err
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.closed {
		return errors.New("shortcut connection is closed")
	}
	if key != 0 {
		var available bool
		if err = k.call("isGlobalShortcutAvailable", key, k.action[0]).Store(&available); err != nil {
			return fmt.Errorf("KDE global shortcuts unavailable: %w", err)
		}
		if !available {
			var owned []int32
			if err = k.call("shortcut", k.action).Store(&owned); err != nil || len(owned) != 1 || owned[0] != key {
				return errors.New("that shortcut is already assigned in KDE; choose another combination")
			}
		}
	}
	if err = k.call("doRegister", k.action).Err; err != nil {
		return fmt.Errorf("register KDE shortcut: %w", err)
	}
	if err = k.set(key); err != nil {
		restore := k.set(k.key)
		if restore != nil {
			k.status(false, restore)
			return fmt.Errorf("%w; restoring previous shortcut also failed: %v", err, restore)
		}
		return err
	}
	k.key = key
	k.value = canonical
	return nil
}
func (k *kdeShortcut) listen() {
	for {
		select {
		case <-k.conn.Context().Done():
			k.mu.Lock()
			closed := k.closed
			k.mu.Unlock()
			if !closed {
				k.status(false, errors.New("global shortcut connection lost; reapply it in Settings"))
			}
			return
		case signal := <-k.signals:
			if signal == nil {
				continue
			}
			if signal.Name == componentInterface+".globalShortcutPressed" && len(signal.Body) >= 2 && signal.Body[0] == k.action[0] && signal.Body[1] == k.action[1] {
				k.mu.Lock()
				enabled := !k.closed && k.key != 0
				k.mu.Unlock()
				if enabled {
					go k.activate()
				}
			}
			if signal.Name == "org.freedesktop.DBus.NameOwnerChanged" && len(signal.Body) == 3 && signal.Body[0] == accelService {
				owner, _ := signal.Body[2].(string)
				if owner == "" {
					k.status(false, errors.New("KDE shortcut service is restarting"))
					continue
				}
				k.mu.Lock()
				value := k.value
				closed := k.closed
				k.mu.Unlock()
				if closed {
					return
				}
				err := k.Apply(value)
				k.status(err == nil && value != "", err)
			}
		}
	}
}
func (k *kdeShortcut) Close() error {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.closed {
		return nil
	}
	k.closed = true
	// Session actions are not saved by KDE. Remove only AFK's action on exit.
	err := k.call("unregister", k.action[0], k.action[1]).Err
	k.conn.RemoveSignal(k.signals)
	_ = k.conn.Close()
	return err
}
func (s *Studio) reportHotkey(active bool, err error) {
	s.mu.Lock()
	s.hotkeyActive = active
	s.hotkeyError = ""
	if err != nil {
		s.hotkeyError = err.Error()
	}
	s.mu.Unlock()
	s.publish()
}
func (s *Studio) startHotkey() {
	s.mu.Lock()
	value := s.saved.Settings.Hotkey
	s.mu.Unlock()
	if err := s.SetHotkey(value); err != nil {
		s.reportHotkey(false, err)
	}
}
func (s *Studio) SetHotkey(value string) error {
	canonical, _, err := parseHotkey(value)
	if err != nil {
		return err
	}
	s.hotkeyMu.Lock()
	defer s.hotkeyMu.Unlock()
	s.mu.Lock()
	old := s.saved.Settings.Hotkey
	closing := s.closing
	s.mu.Unlock()
	if closing {
		return errors.New("AFK is closing")
	}
	if s.shortcut == nil && canonical != "" {
		s.shortcut, err = newKDEShortcut(instanceID, func() {
			if err := s.Toggle(); err != nil {
				s.mu.Lock()
				s.problem = err.Error()
				s.mu.Unlock()
				s.publish()
			}
		}, s.reportHotkey)
		if err != nil {
			return err
		}
	}
	if s.shortcut != nil {
		if err = s.shortcut.Apply(canonical); err != nil {
			return err
		}
	}
	s.mu.Lock()
	s.saved.Settings.Hotkey = canonical
	// Do not rewrite settings on every app start.
	if old != canonical {
		err = s.saveLocked()
	}
	if err != nil {
		s.saved.Settings.Hotkey = old
	}
	s.mu.Unlock()
	if err != nil {
		if s.shortcut != nil {
			if restore := s.shortcut.Apply(old); restore != nil {
				s.reportHotkey(false, restore)
			}
		}
		return err
	}
	s.reportHotkey(canonical != "", nil)
	return nil
}
