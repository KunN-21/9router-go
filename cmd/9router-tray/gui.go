package main

import (
	_ "embed"
	"fmt"
	"os/exec"
	"runtime"
	"time"

	"github.com/getlantern/systray"

	"9router/proxy/internal/tray"
)

//go:embed tray.ico
var trayIcon []byte

// menu holds the items once they are built. systray has no "menu about to
// open" callback, so the menu is created once and then refreshed in place:
// rebuilding is impossible, hiding the stale truth is not.
type menu struct {
	status    *systray.MenuItem
	start     *systray.MenuItem
	stop      *systray.MenuItem
	autostart *systray.MenuItem
}

func runGUI(actions tray.Actions, port int) error {
	systray.Run(func() {
		systray.SetIcon(trayIcon)
		systray.SetTitle("9Router")
		go runMenu(actions, port)
	}, func() {
		// Quit Tray: release the icon and return. Deliberately empty — C4 says
		// quitting the tray must not touch the gateway, and the daemon has
		// already been released by whoever started it, so there is no handle
		// here to close.
	})
	return nil
}

// runMenu builds the items once, then keeps them current.
//
// The refresh loop is the whole point: a tray that draws its state at startup
// and never again says "Running" forever, which is exactly the lie C5 forbids.
// One second is fast enough that a user who starts the gateway from the CLI
// sees the menu agree before they can open it, and cheap enough that reading a
// pid file plus a non-blocking service probe costs nothing.
func runMenu(actions tray.Actions, port int) {
	m := &menu{
		status:    systray.AddMenuItem("9Router", ""),
		start:     systray.AddMenuItem("Start Gateway", ""),
		stop:      systray.AddMenuItem("Stop Gateway", ""),
		autostart: systray.AddMenuItem("Auto-start on Boot", ""),
	}
	m.status.Disable()

	open := systray.AddMenuItem("Open Dashboard", "")
	quit := systray.AddMenuItem("Quit Tray", "")

	go func() {
		for range open.ClickedCh {
			openDashboard(port)
		}
	}()
	go func() {
		for range m.start.ClickedCh {
			report(actions.Start())
			refresh(m, actions)
		}
	}()
	go func() {
		for range m.stop.ClickedCh {
			report(actions.Stop())
			refresh(m, actions)
		}
	}()
	go func() {
		for range m.autostart.ClickedCh {
			// Read again rather than trusting the label: the entry may have
			// been changed by the CLI since this menu was last drawn.
			report(actions.SetAutostart(!actions.AutostartEnabled()))
			refresh(m, actions)
		}
	}()
	go func() {
		for range quit.ClickedCh {
			systray.Quit()
			return
		}
	}()

	refresh(m, actions)
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for range ticker.C {
		refresh(m, actions)
	}
}

// refresh redraws the items from a fresh snapshot. Every branch is a state the
// user can act from, and nothing is inferred past what the snapshot proved.
func refresh(m *menu, actions tray.Actions) {
	snap := actions.Snapshot()
	systray.SetTooltip(snap.Title())
	m.status.SetTitle(snap.Title())

	// Start is offered only from Stopped. Offering it from Unknown is how the
	// user gets ErrAlreadyRunning for a click they did not cause (C5).
	if snap.CanStart() {
		m.start.Show()
		m.start.Enable()
	} else {
		m.start.Hide()
	}

	// Stop is offered only with a verified PID. Whether it will actually run is
	// DecideStop's question, answered on click — the item stays visible so the
	// refusal can be explained rather than the option silently vanishing.
	if snap.CanStop() {
		m.stop.Show()
		m.stop.Enable()
	} else {
		m.stop.Hide()
	}

	label := "Auto-start on Boot"
	if actions.AutostartEnabled() {
		label = "✓ Auto-start on Boot"
	}
	m.autostart.SetTitle(label)
}

// report surfaces a failure without ever printing a credential (C6): errors
// from the actions carry a command and its output, never a token.
func report(err error) {
	if err == nil {
		return
	}
	fmt.Println(err)
}

func openDashboard(port int) {
	url := fmt.Sprintf("http://127.0.0.1:%d/dashboard", port)
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
}
