package api

// Payload field keys and recurring string values shared across REST
// responses, WebSocket event payloads and activity metadata.
// Centralising them keeps the wire vocabulary consistent and typo-safe
// across every handler.
const (
	keyAction        = "action"
	keyAuthenticated = "authenticated"
	keyCreated       = "created"
	keyDeleted       = "deleted"
	keyDirs          = "dirs"
	keyEvent         = "event"
	keyEvents        = "events"
	keyGlobalRev     = "globalRev"
	keyIndex         = "index"
	keyJob           = "job"
	keyJobID         = "jobId"
	keyLauncher      = "launcher"
	keyMessage       = "message"
	keyName          = "name"
	keyOverview      = "overview"
	keyPaneID        = "paneId"
	keyRemoved       = "removed"
	keyRun           = "run"
	keyRunbook       = "runbook"
	keyRunbookID     = "runbookId"
	keyRunbooks      = "runbooks"
	keySchedule      = "schedule"
	keyScheduleID    = "scheduleId"
	keyScope         = "scope"
	keyScript        = "script"
	keyService       = "service"
	keyServices      = "services"
	keySession       = "session"
	keyStatus        = "status"
	keyType          = "type"
)

// Action values carried by the "action" field of event payloads.
const (
	actionSessionCreate = "session.create"
	actionWindowCount   = "window-count"
)
