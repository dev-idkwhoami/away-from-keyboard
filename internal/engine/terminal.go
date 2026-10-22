package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// patchTerminal gives the native microphone example a bounded one-line view.
// Audio-window resets still happen at the original interval, independently of
// terminal output. Matching exact pinned source prevents silently partial edits.
func patchTerminal(source, path string) error {
	if err := os.WriteFile(filepath.Join(source, "examples", "stream", "afk-terminal.h"), terminalHeader, 0644); err != nil {
		return err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	s := string(b)
	// The desktop reads a pipe, so announce readiness without waiting for speech.
	if !strings.Contains(s, `printf("[Start speaking]\n"); std::fflush(stdout);`) {
		s = strings.Replace(s, `printf("[Start speaking]\n");`, `printf("[Start speaking]\n"); std::fflush(stdout);`, 1)
	}
	if strings.Contains(s, "// AFK single-line output") {
		return os.WriteFile(path, []byte(s), 0644)
	}
	start := strings.Index(s, "            // print result;")
	end := strings.Index(s, "            ++n_iter;")
	if start < 0 || end <= start {
		return fmt.Errorf("unexpected upstream stream output block")
	}
	s = s[:start] + `            // AFK single-line output: each result replaces the provisional tail.
            std::string text;
            for (int i = 0; i < whisper_full_n_segments(ctx); ++i) {
                text += whisper_full_get_segment_text(ctx, i);
            }
            afk_line.update(text);

` + s[end:]
	replacements := [][2]string{
		{"#include \"whisper.h\"", "#include \"whisper.h\"\n#include \"afk-terminal.h\""},
		{"    int n_iter = 0;", `    afk_terminal::line afk_line;
    // Auto-language messages are generated each refresh; keep them off the row.
    whisper_log_set([](ggml_log_level, const char * text, void *) {
        if (std::strstr(text, "auto-detected language:") == nullptr) {
            std::fputs(text, stderr);
        }
    }, nullptr);
    int n_iter = 0;`},
		{"if (!use_vad && (n_iter % n_new_line) == 0) {\n                printf(\"\\n\");", "if (!use_vad && (n_iter % n_new_line) == 0) {\n                afk_line.commit();"},
		{"    audio.pause();", "    std::printf(\"\\n\");\n    audio.pause();"},
	}
	for _, p := range replacements {
		if strings.Count(s, p[0]) != 1 {
			return fmt.Errorf("unexpected upstream terminal patch anchor %q", p[0])
		}
		s = strings.Replace(s, p[0], p[1], 1)
	}
	return os.WriteFile(path, []byte(s), 0644)
}
