package validate

import "regexp"

var sessionNameRE = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)

// SessionName reports whether name is a valid tmux session name.
func SessionName(name string) bool {
	return sessionNameRE.MatchString(name)
}

var iconKeyRE = regexp.MustCompile(`^[a-z0-9-]{1,32}$`)

// IconKey reports whether key is a valid session icon key.
func IconKey(key string) bool {
	return iconKeyRE.MatchString(key)
}
