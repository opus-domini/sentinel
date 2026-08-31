package store

// Field keys used when building watchtower projection payloads and the
// column name shared across watchtower tables.
const (
	wtKeySession     = "session"
	wtKeyPanes       = "panes"
	wtColSessionName = "session_name"
	// wtColName is the sessions table's own session-name column.
	wtColName = "name"
)
