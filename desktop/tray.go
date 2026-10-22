package main

import "errors"

func (s *Studio) SelectModel(id string) error {
	state := s.GetState()
	installed := false
	for _, model := range state.Models {
		if model.ID == id && model.Installed {
			installed = true
		}
	}
	if !installed {
		return errors.New("download this model in Model Garage first")
	}
	settings := state.Settings
	settings.Model = id
	return s.SetSettings(settings)
}
func (s *Studio) trayAction(action func() error) {
	if err := action(); err != nil {
		s.mu.Lock()
		s.problem = err.Error()
		s.mu.Unlock()
		s.OpenStudio()
	}
	s.publish()
}
func (s *Studio) updateTray(state State) {
	ready := (state.Status == "idle" || state.Status == "error") && state.Preparing == ""
	if s.trayFile != nil {
		s.trayFile.SetEnabled(ready)
	}
	for _, model := range state.Models {
		if item := s.trayModels[model.ID]; item != nil {
			item.SetChecked(state.Settings.Model == model.ID)
			item.SetEnabled(ready && model.Installed)
		}
	}
}
