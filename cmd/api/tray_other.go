//go:build !windows && !darwin

package main

import (
	"log"
	"os/exec"
)

// runTray implements "-tray" (desktop launch mode) on platforms without a native
// tray implementation (Linux/BSD). There's no tray icon; the server runs in a
// background goroutine, so we open the browser once and then block to keep the
// process alive.
func runTray(url string) {
	log.Printf("running (no tray on this platform); serving at %s", url)
	openBrowser(url)
	select {}
}

// openBrowser opens the default browser at url (used by single-instance handling).
func openBrowser(url string) {
	// Run (not Start) and wait: in the single-instance path the process exits
	// right after this, so the launch must complete before we return.
	if err := exec.Command("xdg-open", url).Run(); err != nil {
		log.Printf("open browser: %v", err)
	}
}
