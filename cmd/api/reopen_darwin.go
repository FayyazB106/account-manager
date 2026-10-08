//go:build darwin

package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa
#import <Cocoa/Cocoa.h>

extern void goReopen(void);

// fyne.io/systray installs a SystrayAppDelegate as the NSApplication delegate but
// doesn't handle the "reopen" event — the one macOS sends when the app's icon is
// clicked while it's already running (macOS activates the existing process rather
// than launching a new one, so our single-instance check never runs). Add the
// handler via a category so that click opens the app in the browser, matching the
// Windows tray behaviour.
@interface SystrayAppDelegate : NSObject
@end

@implementation SystrayAppDelegate (Reopen)
- (BOOL)applicationShouldHandleReopen:(NSApplication *)sender hasVisibleWindows:(BOOL)flag {
    goReopen();
    return YES;
}
@end
*/
import "C"

//export goReopen
func goReopen() {
	// Off the Cocoa main thread so launching the browser can't stall the UI.
	go reopenBrowser()
}
