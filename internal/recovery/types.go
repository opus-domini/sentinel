package recovery

import "time"

type ReplayMode string

const (
	ReplayModeSafe    ReplayMode = "safe"
	ReplayModeConfirm ReplayMode = "confirm"
	ReplayModeFull    ReplayMode = "full"
)

type ConflictPolicy string

const (
	ConflictRename  ConflictPolicy = "rename"
	ConflictReplace ConflictPolicy = "replace"
	ConflictSkip    ConflictPolicy = "skip"
)

type WindowSnapshot struct {
	Index  int    `json:"index"`
	Name   string `json:"name"`
	Active bool   `json:"active"`
	Panes  int    `json:"panes"`
	Layout string `json:"layout"`
}

type PaneSnapshot struct {
	WindowIndex    int    `json:"windowIndex"`
	PaneIndex      int    `json:"paneIndex"`
	Title          string `json:"title"`
	Active         bool   `json:"active"`
	CurrentPath    string `json:"currentPath"`
	StartCommand   string `json:"startCommand"`
	CurrentCommand string `json:"currentCommand"`
	LastContent    string `json:"lastContent"`
}

type SessionSnapshot struct {
	SessionName  string           `json:"sessionName"`
	CapturedAt   time.Time        `json:"capturedAt"`
	BootID       string           `json:"bootId"`
	Attached     int              `json:"attached"`
	ActiveWindow int              `json:"activeWindow"`
	ActivePaneID string           `json:"activePaneId"`
	Windows      []WindowSnapshot `json:"windows"`
	Panes        []PaneSnapshot   `json:"panes"`
}

type RestoreOptions struct {
	Mode           ReplayMode     `json:"mode"`
	ConflictPolicy ConflictPolicy `json:"conflictPolicy"`
	TargetSession  string         `json:"targetSession"`
}

func (o RestoreOptions) normalize() RestoreOptions {
	out := o
	switch out.Mode {
	case ReplayModeSafe, ReplayModeConfirm, ReplayModeFull:
	default:
		out.Mode = ReplayModeConfirm
	}
	switch out.ConflictPolicy {
	case ConflictRename, ConflictReplace, ConflictSkip:
	default:
		out.ConflictPolicy = ConflictRename
	}
	return out
}
