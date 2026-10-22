// Build-time utility, never installed in the end-user package.
package main

import (
	"afk/internal/engine"
	"context"
	"encoding/json"
	"flag"
	"log"
	"os"
	"os/signal"
)

func main() {
	root := flag.String("data", "work/package-runtime", "native build cache")
	arch := flag.String("cuda-arch", "75", "minimum CUDA architecture, includes forward-compatible PTX")
	jobs := flag.Int("jobs", 2, "parallel compiler jobs")
	flag.Parse()
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	whisper, nemo, err := engine.BuildPackageRuntimes(ctx, *root, *arch, *jobs)
	if err != nil {
		log.Fatal(err)
	}
	if err = json.NewEncoder(os.Stdout).Encode(map[string]string{"whisper": whisper, "nemotron": nemo}); err != nil {
		log.Fatal(err)
	}
}
