package main

import "github.com/wailsapp/wails/v3/pkg/application"

var bubblePositions = map[string][2]int{
	"top-left": {0, 0}, "top-center": {1, 0}, "top-right": {2, 0},
	"middle-left": {0, 1}, "center": {1, 1}, "middle-right": {2, 1},
	"bottom-left": {0, 2}, "bottom-center": {1, 2}, "bottom-right": {2, 2},
}

func bubbleCoordinates(area application.Rect, width, height int, position string) (int, int) {
	anchor := bubblePositions[position]
	coordinate := func(origin, span, size, align int) int {
		available := max(0, span-size)
		inset := min(20, available/2)
		switch align {
		case 0:
			return origin + inset
		case 2:
			return origin + available - inset
		default:
			return origin + available/2
		}
	}
	return coordinate(area.X, area.Width, width, anchor[0]), coordinate(area.Y, area.Height, height, anchor[1])
}
func (s *Studio) placeBubble(width, height int, position string) {
	if s.app == nil || s.bubble == nil {
		return
	}
	screen := s.app.Screen.GetPrimary()
	if screen == nil {
		return
	}
	x, y := bubbleCoordinates(screen.WorkArea, width, height, position)
	s.bubble.SetPosition(x, y)
}
