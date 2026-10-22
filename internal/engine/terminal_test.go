package engine

import (
	"encoding/json"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestNativeTerminal(t *testing.T) {
	compiler, err := exec.LookPath("c++")
	if err != nil {
		t.Skip("native terminal test requires C++ compiler")
	}
	bin := filepath.Join(t.TempDir(), "terminal-test")
	if out, err := exec.Command(compiler, "-std=c++17", "native/terminal_test.cpp", "-lutil", "-o", bin).CombinedOutput(); err != nil {
		t.Fatalf("compile: %v\n%s", err, out)
	}
	if out, err := exec.Command(bin).CombinedOutput(); err != nil {
		t.Fatalf("terminal regression: %v\n%s", err, out)
	}
	out, err := exec.Command(bin, "events").Output()
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 events: %s", out)
	}
	for i, line := range lines {
		var event struct {
			Type string
			Text string
		}
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatal(err)
		}
		wantType, wantText := "partial", `Grüße "AFK" \ test`
		if i == 1 {
			wantType = "commit"
		}
		if i == 2 {
			wantText = "next"
		}
		if event.Type != wantType || event.Text != wantText {
			t.Fatalf("unexpected event: %+v", event)
		}
	}

}
