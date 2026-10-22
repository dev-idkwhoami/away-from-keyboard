//go:build linux && gtk3

package main

/*
#cgo pkg-config: gtk+-3.0
#include <gtk/gtk.h>

static void afk_bubble_children(GtkWidget *widget, gpointer data) {
    // WebKit's natural minimum is 200px. Explicit requisitions allow a notch.
    gtk_widget_set_size_request(widget, 1, 1);
    if (GTK_IS_CONTAINER(widget)) gtk_container_foreach(GTK_CONTAINER(widget), afk_bubble_children, NULL);
}
static void afk_prepare_bubble(GtkWindow *window) {
    // Opt this window into client-side decorations on Wayland, then remove
    // them. This avoids a compositor-supplied title bar for the overlay.
    if (!gtk_widget_get_realized(GTK_WIDGET(window))) {
        GtkWidget *titlebar = gtk_box_new(GTK_ORIENTATION_HORIZONTAL, 0);
        gtk_window_set_titlebar(window, titlebar);
    }
    gtk_window_set_decorated(window, FALSE);
    gtk_container_foreach(GTK_CONTAINER(window), afk_bubble_children, NULL);
}
static void afk_size_bubble(GtkWindow *window, int width, int height) {
    // GTK ignores resize requests for non-resizable windows in some backends.
    // Lock the geometry through equal min/max hints instead.
    gtk_window_set_resizable(window, TRUE);
    GdkGeometry geometry = {0};
    geometry.min_width = geometry.max_width = width;
    geometry.min_height = geometry.max_height = height;
    gtk_window_set_geometry_hints(window, NULL, &geometry, GDK_HINT_MIN_SIZE | GDK_HINT_MAX_SIZE);
    gtk_window_set_default_size(window, width, height);
    gtk_window_resize(window, width, height);
}
*/
import "C"
import "github.com/wailsapp/wails/v3/pkg/application"

func prepareBubbleWindow(w *application.WebviewWindow) {
	application.InvokeSync(func() {
		if native := w.NativeWindow(); native != nil {
			C.afk_prepare_bubble((*C.GtkWindow)(native))
		}
	})
}
func sizeBubbleWindow(w *application.WebviewWindow, width, height int) {
	application.InvokeSync(func() {
		if native := w.NativeWindow(); native != nil {
			C.afk_size_bubble((*C.GtkWindow)(native), C.int(width), C.int(height))
		}
	})
}
