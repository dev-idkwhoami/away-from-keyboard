package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

const defaultMicrophone = "@DEFAULT_SOURCE@"
const defaultMeetingAudio = "@DEFAULT_MONITOR@"

type AudioSource struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	Kind   string `json:"kind"`
	Device string `json:"-"`
	Stream uint32 `json:"-"`
}
type audioNode struct {
	Index       uint32            `json:"index"`
	Sink        uint32            `json:"sink"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Monitor     string            `json:"monitor_source"`
	Properties  map[string]string `json:"properties"`
}

func audioInventory(ctx context.Context) ([]AudioSource, error) {
	get := func(kind string) ([]audioNode, error) {
		out, err := exec.CommandContext(ctx, "pactl", "-f", "json", "list", kind).Output()
		if err != nil {
			return nil, fmt.Errorf("list %s: %w", kind, err)
		}
		var nodes []audioNode
		err = json.Unmarshal(out, &nodes)
		return nodes, err
	}
	sources, err := get("sources")
	if err != nil {
		return nil, err
	}
	sinks, err := get("sinks")
	if err != nil {
		return nil, err
	}
	apps, err := get("sink-inputs")
	if err != nil {
		return nil, err
	}
	result := []AudioSource{}
	for _, source := range sources {
		kind := "mic"
		if source.Properties["device.class"] == "monitor" || strings.HasSuffix(source.Name, ".monitor") {
			kind = "output"
		}
		result = append(result, AudioSource{ID: source.Name, Label: source.Description, Kind: kind, Device: source.Name})
	}
	for _, app := range apps {
		if strings.HasPrefix(app.Properties["application.name"], "AFK") {
			continue
		}
		for _, sink := range sinks {
			if sink.Index == app.Sink {
				serial := app.Properties["object.serial"]
				if serial == "" {
					serial = app.Properties["application.process.id"]
				}
				label := app.Properties["application.name"]
				if label == "" {
					label = "Application"
				}
				label += " · " + app.Properties["media.name"]
				result = append(result, AudioSource{ID: "app:" + strconv.FormatUint(uint64(app.Index), 10) + ":" + serial, Label: label, Kind: "app", Device: sink.Monitor, Stream: app.Index})
			}
		}
	}
	out, err := exec.CommandContext(ctx, "pactl", "-f", "json", "info").Output()
	if err != nil {
		return nil, fmt.Errorf("read default audio devices: %w", err)
	}
	var defaults struct {
		Source string `json:"default_source_name"`
		Sink   string `json:"default_sink_name"`
	}
	if err = json.Unmarshal(out, &defaults); err != nil {
		return nil, err
	}
	return withAudioDefaults(result, sinks, defaults.Source, defaults.Sink), nil
}

func withAudioDefaults(sources []AudioSource, sinks []audioNode, sourceName, sinkName string) []AudioSource {
	mic := AudioSource{ID: defaultMicrophone, Label: "System default microphone (unavailable)", Kind: "mic"}
	output := AudioSource{ID: defaultMeetingAudio, Label: "System default output (unavailable)", Kind: "output"}
	for _, source := range sources {
		if source.ID == sourceName && source.Kind == "mic" {
			mic.Device = source.Device
			mic.Label = "System default microphone · " + source.Label
		}
	}
	for _, sink := range sinks {
		if sink.Name == sinkName {
			for _, source := range sources {
				if source.ID == sink.Monitor && source.Kind == "output" {
					output.Device = source.Device
					output.Label = "System default output · " + sink.Description
				}
			}
		}
	}
	return append([]AudioSource{mic, output}, sources...)
}
func (s *Studio) GetAudioSources() ([]AudioSource, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	return audioInventory(ctx)
}
