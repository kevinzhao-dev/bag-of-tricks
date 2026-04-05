package pv

import (
	"encoding/json"
	"fmt"
	"sync"
)

// CommandMessage is received from the browser.
type CommandMessage struct {
	Cmd   string `json:"cmd"`
	Pane  int    `json:"pane,omitempty"`
	Delta int    `json:"delta,omitempty"`
	Index int    `json:"index,omitempty"`
}

// App is the core photo viewer application.
type App struct {
	State *ViewerState

	mu          sync.Mutex
	subscribers map[chan StateSnapshot]struct{}
	quit        chan struct{}
}

// NewApp creates a new App with the given image list.
func NewApp(images []string) *App {
	return &App{
		State:       NewViewerState(images),
		subscribers: make(map[chan StateSnapshot]struct{}),
		quit:        make(chan struct{}),
	}
}

// HandleCommand processes a command from the browser and broadcasts the new state.
func (a *App) HandleCommand(data []byte) {
	var cmd CommandMessage
	if err := json.Unmarshal(data, &cmd); err != nil {
		return
	}

	switch cmd.Cmd {
	case "next":
		a.State.NextInPane()
	case "prev":
		a.State.PrevInPane()
	case "grid_up":
		a.State.GridUp()
	case "grid_down":
		a.State.GridDown()
	case "focus":
		a.State.SetFocus(cmd.Pane)
	case "focus_next":
		a.State.FocusNext()
	case "focus_prev":
		a.State.FocusPrev()
	case "jump":
		a.State.JumpInPane(cmd.Delta)
	case "goto":
		a.State.GotoInPane(cmd.Index)
	case "next_dir":
		a.State.NextDir()
	case "prev_dir":
		a.State.PrevDir()
	case "advance_all":
		a.State.AdvanceAll()
	case "retreat_all":
		a.State.RetreatAll()
	case "enter_speed":
		a.State.EnterSpeedMode()
	case "exit_speed":
		a.State.ExitSpeedMode()
	case "quit":
		select {
		case <-a.quit:
		default:
			close(a.quit)
		}
		return
	default:
		return
	}

	a.Broadcast()
}

// Subscribe returns a channel that receives state snapshots.
func (a *App) Subscribe() chan StateSnapshot {
	a.mu.Lock()
	defer a.mu.Unlock()
	ch := make(chan StateSnapshot, 4)
	a.subscribers[ch] = struct{}{}
	return ch
}

// Unsubscribe removes a subscriber channel.
func (a *App) Unsubscribe(ch chan StateSnapshot) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.subscribers, ch)
	close(ch)
}

// Broadcast sends the current state to all subscribers.
func (a *App) Broadcast() {
	a.mu.Lock()
	defer a.mu.Unlock()
	snap := a.State.Snapshot()
	for ch := range a.subscribers {
		select {
		case ch <- snap:
		default:
			// drop if subscriber is slow
		}
	}
}

// Quit returns the quit channel.
func (a *App) Quit() <-chan struct{} {
	return a.quit
}

// ImagePath returns the absolute file path for image at index idx.
func (a *App) ImagePath(idx int) (string, error) {
	a.State.mu.RLock()
	defer a.State.mu.RUnlock()
	if idx < 0 || idx >= len(a.State.images) {
		return "", fmt.Errorf("index out of range: %d", idx)
	}
	return a.State.images[idx], nil
}
