package engine

import (
	"encoding/json"
	"os"
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

// Exercise the native silence branch with several phrases separated by quiet
// steps. The desktop replaces partials, so each earlier phrase must be committed.
func TestSilencePreservesEarlierPhrases(t *testing.T) {
	compiler, err := exec.LookPath("c++")
	if err != nil {
		t.Skip("requires C++ compiler")
	}
	dir := t.TempDir()
	header, err := filepath.Abs("native/terminal.h")
	if err != nil {
		t.Fatal(err)
	}
	source := `#include "` + header + `"
int main() {
 setenv("AFK_EVENTS", "1", 1);
 afk_terminal::line afk_line;
 bool use_vad = false;
 int n_iter = 5;
 std::vector<float> pcmf32_old(16000, 0.1f);
 std::vector<int> prompt_tokens{1, 2};
 const double gate_level = 0.003;
 for (int step = 0; step < 5; ++step) {
   double rms = step % 2 ? 0.0 : 0.1;
   ` + silenceBoundary + `
   if (step > 0 && (!pcmf32_old.empty() || !prompt_tokens.empty() || n_iter != 0)) return 2;
   afk_line.update(step == 0 ? "First sentence." : step == 2 ? "Second sentence." : "Last sentence.");
 }
}
`
	path := filepath.Join(dir, "pause.cpp")
	if err := os.WriteFile(path, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(dir, "pause")
	if out, err := exec.Command(compiler, "-std=c++17", path, "-o", bin).CombinedOutput(); err != nil {
		t.Fatalf("compile: %v %s", err, out)
	}
	out, err := exec.Command(bin).Output()
	if err != nil {
		t.Fatal(err)
	}
	committed, pending := "", ""
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		var event struct{ Type, Text string }
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatal(err)
		}
		switch event.Type {
		case "partial":
			pending = event.Text
		case "commit":
			committed += event.Text + " "
			pending = ""
		}
	}
	if got := committed + pending; got != "First sentence. Second sentence. Last sentence." {
		t.Fatalf("lost speech: %q", got)
	}
}

func TestPatchExistingStreamSilenceBoundary(t *testing.T) {
	dir := t.TempDir()
	streamDir := filepath.Join(dir, "examples", "stream")
	if err := os.MkdirAll(streamDir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(streamDir, "stream.cpp")
	original := "// AFK single-line output\nif (rms < gate_level) { continue; }\n"
	if err := os.WriteFile(path, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := patchTerminal(dir, path); err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Count(string(got), silenceBoundary) != 1 {
			t.Fatalf("missing or duplicated silence boundary: %s", got)
		}
	}
}
