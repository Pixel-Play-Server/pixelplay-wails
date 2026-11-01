//go:build linux

package webkitperm

/*
#cgo !webkit2_41 pkg-config: webkit2gtk-4.0
#cgo webkit2_41 pkg-config: webkit2gtk-4.1

#include <gtk/gtk.h>
#include <webkit2/webkit2.h>

static gboolean pp_on_permission_request(WebKitWebView *web_view, WebKitPermissionRequest *request, gpointer user_data) {
	(void)web_view;
	(void)user_data;
	// WebKitGTK denies unhandled permission-request → getUserMedia NotAllowedError.
	if (WEBKIT_IS_USER_MEDIA_PERMISSION_REQUEST(request)) {
		webkit_permission_request_allow(request);
		return TRUE;
	}
	return FALSE;
}

static void pp_walk_connect(GtkWidget *widget, gpointer data) {
	if (WEBKIT_IS_WEB_VIEW(widget)) {
        WebKitSettings *settings = webkit_web_view_get_settings(WEBKIT_WEB_VIEW(widget));
        webkit_settings_set_enable_media_stream(settings, TRUE);
        webkit_settings_set_enable_webrtc(settings, TRUE);
        webkit_settings_set_media_playback_allows_inline(settings, TRUE);
        webkit_settings_set_media_playback_requires_user_gesture(webkit_web_view_get_settings(WEBKIT_WEB_VIEW(widget)), FALSE);
		if (g_object_get_data(G_OBJECT(widget), "pp-media-perm") == NULL) {
			g_object_set_data(G_OBJECT(widget), "pp-media-perm", GINT_TO_POINTER(1));
			g_signal_connect(widget, "permission-request", G_CALLBACK(pp_on_permission_request), NULL);
			*((int *)data) = *((int *)data) + 1;
		}
	}
	if (GTK_IS_CONTAINER(widget)) {
		gtk_container_forall(GTK_CONTAINER(widget), pp_walk_connect, data);
	}
}

static int pp_connect_all(void) {
	int connected = 0;
	GList *tops = gtk_window_list_toplevels();
	for (GList *l = tops; l != NULL; l = l->next) {
		pp_walk_connect(GTK_WIDGET(l->data), &connected);
	}
	g_list_free(tops);
	return connected;
}

static gboolean pp_idle_connect(gpointer data) {
	(void)data;
	(void)pp_connect_all();
	return G_SOURCE_REMOVE;
}

static int pp_retry_left = 40;

static gboolean pp_retry_connect(gpointer data) {
	(void)data;
	if (pp_connect_all() > 0) {
		pp_retry_left = 0;
		return G_SOURCE_REMOVE;
	}
	pp_retry_left--;
	return pp_retry_left > 0 ? G_SOURCE_CONTINUE : G_SOURCE_REMOVE;
}

void pp_webkit_install_user_media_permission(void) {
	g_idle_add(pp_idle_connect, NULL);
	// Webview may appear a bit after OnDomReady — keep trying ~10s.
	pp_retry_left = 40;
	g_timeout_add(250, pp_retry_connect, NULL);
}
*/
import "C"

// Install hooks WebKitGTK permission-request so getUserMedia (mic/cam) is allowed.
// Without this, Linux Wails silently denies every media permission.
func Install() {
	C.pp_webkit_install_user_media_permission()
}
