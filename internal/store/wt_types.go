package store

import "time"

type WatchtowerSession struct {
	SessionName       string    `json:"sessionName"`
	Attached          int       `json:"attached"`
	Windows           int       `json:"windows"`
	Panes             int       `json:"panes"`
	ActivityAt        time.Time `json:"activityAt"`
	LastPreview       string    `json:"lastPreview"`
	LastPreviewAt     time.Time `json:"lastPreviewAt"`
	LastPreviewPaneID string    `json:"lastPreviewPaneId"`
	UnreadWindows     int       `json:"unreadWindows"`
	UnreadPanes       int       `json:"unreadPanes"`
	Rev               int64     `json:"rev"`
	UpdatedAt         time.Time `json:"updatedAt"`
}

type WatchtowerSessionWrite struct {
	SessionName       string
	Attached          int
	Windows           int
	Panes             int
	ActivityAt        time.Time
	LastPreview       string
	LastPreviewAt     time.Time
	LastPreviewPaneID string
	UnreadWindows     int
	UnreadPanes       int
	Rev               int64
	UpdatedAt         time.Time
}

type WatchtowerWindow struct {
	SessionName      string    `json:"sessionName"`
	TmuxWindowID     string    `json:"tmuxWindowId"`
	WindowIndex      int       `json:"windowIndex"`
	Name             string    `json:"name"`
	Active           bool      `json:"active"`
	Layout           string    `json:"layout"`
	WindowActivityAt time.Time `json:"windowActivityAt"`
	UnreadPanes      int       `json:"unreadPanes"`
	HasUnread        bool      `json:"hasUnread"`
	Rev              int64     `json:"rev"`
	UpdatedAt        time.Time `json:"updatedAt"`
}

type WatchtowerWindowWrite struct {
	SessionName      string
	TmuxWindowID     string
	WindowIndex      int
	Name             string
	Active           bool
	Layout           string
	WindowActivityAt time.Time
	UnreadPanes      int
	HasUnread        bool
	Rev              int64
	UpdatedAt        time.Time
}

type WatchtowerPane struct {
	PaneID         string    `json:"paneId"`
	SessionName    string    `json:"sessionName"`
	WindowIndex    int       `json:"windowIndex"`
	PaneIndex      int       `json:"paneIndex"`
	Title          string    `json:"title"`
	Active         bool      `json:"active"`
	TTY            string    `json:"tty"`
	CurrentPath    string    `json:"currentPath"`
	StartCommand   string    `json:"startCommand"`
	CurrentCommand string    `json:"currentCommand"`
	TailHash       string    `json:"tailHash"`
	TailPreview    string    `json:"tailPreview"`
	TailCapturedAt time.Time `json:"tailCapturedAt"`
	Revision       int64     `json:"revision"`
	SeenRevision   int64     `json:"seenRevision"`
	ChangedAt      time.Time `json:"changedAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

type WatchtowerPaneWrite struct {
	PaneID         string
	SessionName    string
	WindowIndex    int
	PaneIndex      int
	Title          string
	Active         bool
	TTY            string
	CurrentPath    string
	StartCommand   string
	CurrentCommand string
	TailHash       string
	TailPreview    string
	TailCapturedAt time.Time
	Revision       int64
	SeenRevision   int64
	ChangedAt      time.Time
	UpdatedAt      time.Time
}

type WatchtowerPresence struct {
	TerminalID  string    `json:"terminalId"`
	SessionName string    `json:"sessionName"`
	WindowIndex int       `json:"windowIndex"`
	PaneID      string    `json:"paneId"`
	Visible     bool      `json:"visible"`
	Focused     bool      `json:"focused"`
	UpdatedAt   time.Time `json:"updatedAt"`
	ExpiresAt   time.Time `json:"expiresAt"`
}

type WatchtowerPresenceWrite struct {
	TerminalID  string
	SessionName string
	WindowIndex int
	PaneID      string
	Visible     bool
	Focused     bool
	UpdatedAt   time.Time
	ExpiresAt   time.Time
}

type WatchtowerJournal struct {
	ID         int64     `json:"id"`
	GlobalRev  int64     `json:"globalRev"`
	EntityType string    `json:"entityType"`
	Session    string    `json:"session"`
	WindowIdx  int       `json:"windowIndex"`
	PaneID     string    `json:"paneId"`
	ChangeKind string    `json:"changeKind"`
	ChangedAt  time.Time `json:"changedAt"`
}

type WatchtowerJournalWrite struct {
	GlobalRev  int64
	EntityType string
	Session    string
	WindowIdx  int
	PaneID     string
	ChangeKind string
	ChangedAt  time.Time
}
