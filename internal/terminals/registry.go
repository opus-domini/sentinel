package terminals

import (
	"crypto/rand"
	"encoding/hex"
	"sort"
	"strconv"
	"sync"
	"time"
)

type Snapshot struct {
	ID         string    `json:"id"`
	Session    string    `json:"session"`
	RemoteAddr string    `json:"remoteAddr"`
	StartedAt  time.Time `json:"startedAt"`
	Cols       int       `json:"cols"`
	Rows       int       `json:"rows"`
}

type entry struct {
	id         string
	session    string
	remoteAddr string
	startedAt  time.Time
	cols       int
	rows       int
	close      func(reason string)
}

type Registry struct {
	mu      sync.RWMutex
	entries map[string]*entry
}

func NewRegistry() *Registry {
	return &Registry{
		entries: make(map[string]*entry),
	}
}

func (r *Registry) Register(session, remoteAddr string, cols, rows int, closeFn func(reason string)) (string, func()) {
	if r == nil {
		return "", func() {}
	}

	id := randomID()
	newEntry := &entry{
		id:         id,
		session:    session,
		remoteAddr: remoteAddr,
		startedAt:  time.Now().UTC(),
		cols:       cols,
		rows:       rows,
		close:      closeFn,
	}

	r.mu.Lock()
	r.entries[id] = newEntry
	r.mu.Unlock()

	var once sync.Once
	release := func() {
		once.Do(func() {
			r.mu.Lock()
			delete(r.entries, id)
			r.mu.Unlock()
		})
	}

	return id, release
}

func (r *Registry) UpdateSize(id string, cols, rows int) {
	if r == nil || id == "" || cols <= 0 || rows <= 0 {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	current, ok := r.entries[id]
	if !ok {
		return
	}
	current.cols = cols
	current.rows = rows
}

func (r *Registry) List() []Snapshot {
	if r == nil {
		return []Snapshot{}
	}

	r.mu.RLock()
	out := make([]Snapshot, 0, len(r.entries))
	for _, item := range r.entries {
		out = append(out, Snapshot{
			ID:         item.id,
			Session:    item.session,
			RemoteAddr: item.remoteAddr,
			StartedAt:  item.startedAt,
			Cols:       item.cols,
			Rows:       item.rows,
		})
	}
	r.mu.RUnlock()

	sort.Slice(out, func(i, j int) bool {
		if out[i].StartedAt.Equal(out[j].StartedAt) {
			return out[i].ID < out[j].ID
		}
		return out[i].StartedAt.After(out[j].StartedAt)
	})

	return out
}

func (r *Registry) Close(id, reason string) bool {
	if r == nil || id == "" {
		return false
	}

	r.mu.RLock()
	current, ok := r.entries[id]
	r.mu.RUnlock()
	if !ok {
		return false
	}

	if current.close != nil {
		current.close(reason)
	}
	return true
}

func randomID() string {
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err == nil {
		return hex.EncodeToString(raw[:])
	}
	return strconv.FormatInt(time.Now().UnixNano(), 16)
}
