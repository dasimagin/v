package main

/*
#cgo darwin CFLAGS: -x objective-c
#cgo darwin LDFLAGS: -framework Cocoa
#import <Cocoa/Cocoa.h>

// Must run on the main thread.
static void setChrome(void *win, int dark, int r, int g, int b) {
	NSWindow *w = (NSWindow *)win;
	w.appearance = [NSAppearance appearanceNamed:dark ? NSAppearanceNameDarkAqua : NSAppearanceNameAqua];
	w.styleMask |= NSWindowStyleMaskFullSizeContentView; // the page runs under the title bar
	w.titlebarAppearsTransparent = YES;
	w.titleVisibility = NSWindowTitleHidden; // the page has its own header
	w.backgroundColor = [NSColor colorWithSRGBRed:r/255.0 green:g/255.0 blue:b/255.0 alpha:1];
}

static void dragWindow(void *win) {
	NSEvent *e = [NSApp currentEvent];
	if (e) [(NSWindow *)win performWindowDragWithEvent:e];
}

static void zoomWindow(void *win) { [(NSWindow *)win performZoom:nil]; }

// Where the window buttons are: the center of the row from the top, and the
// right edge of the last button, both in points.
static void barMetrics(void *win, double *cy, double *right) {
	NSWindow *w = (NSWindow *)win;
	NSButton *close = [w standardWindowButton:NSWindowCloseButton];
	NSButton *zoom = [w standardWindowButton:NSWindowZoomButton];
	if (!close || !zoom) { *cy = 0; *right = 0; return; }
	NSRect c = [close convertRect:close.bounds toView:nil];
	NSRect z = [zoom convertRect:zoom.bounds toView:nil];
	*cy = w.frame.size.height - NSMidY(c);
	*right = NSMaxX(z);
}

static void setAppIcon(const void *bytes, int n) {
	NSData *d = [NSData dataWithBytes:bytes length:n];
	NSImage *img = [[NSImage alloc] initWithData:d];
	if (img) [NSApp setApplicationIconImage:img];
}
*/
import "C"

import (
	"strconv"
	"strings"
	"unsafe"
)

// setWindowChrome makes the title bar and the window background match the
// page: dark or light, with the page's background color ("#rrggbb").
func setWindowChrome(win unsafe.Pointer, dark bool, bg string) {
	v, err := strconv.ParseUint(strings.TrimPrefix(bg, "#"), 16, 32)
	if err != nil {
		return
	}
	d := C.int(0)
	if dark {
		d = 1
	}
	C.setChrome(win, d, C.int(v>>16&0xff), C.int(v>>8&0xff), C.int(v&0xff))
}

// dragWindow starts moving the window with the mouse that is held down now.
func dragWindow(win unsafe.Pointer) { C.dragWindow(win) }

// zoomWindow maximizes the window or restores it.
func zoomWindow(win unsafe.Pointer) { C.zoomWindow(win) }

// windowBarMetrics reports where the window buttons are, so the page's header
// can line up with them: the vertical center and the right edge of the last one.
func windowBarMetrics(win unsafe.Pointer) (cy, right float64) {
	var c, r C.double
	C.barMetrics(win, &c, &r)
	return float64(c), float64(r)
}

// setAppIcon replaces the Dock icon with a PNG. It must run on the main thread.
func setAppIcon(png []byte) {
	if len(png) > 0 {
		C.setAppIcon(unsafe.Pointer(&png[0]), C.int(len(png)))
	}
}
