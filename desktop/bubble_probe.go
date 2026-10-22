//go:build smoke && linux && gtk3

package main

/*
#cgo pkg-config: gtk+-3.0
#include <gtk/gtk.h>
*/
import "C"
import (
	"fmt"
	"github.com/wailsapp/wails/v3/pkg/application"
)

func probeBubble(w *application.WebviewWindow, wantWidth, wantHeight int) error {
	var err error
	application.InvokeSync(func() {
		win := (*C.GtkWindow)(w.NativeWindow())
		var width, height C.gint
		C.gtk_window_get_size(win, &width, &height)
		decorated := C.gtk_window_get_decorated(win) != 0
		fmt.Printf("BUBBLE NATIVE: title=%s decorated=%v size=%dx%d\n", C.GoString(C.gtk_window_get_title(win)), decorated, int(width), int(height))
		if int(width) != wantWidth || int(height) != wantHeight {
			err = fmt.Errorf("bubble size %dx%d, want %dx%d", int(width), int(height), wantWidth, wantHeight)
		}
		if decorated {
			err = fmt.Errorf("bubble has native decorations")
		}
	})
	return err
}
