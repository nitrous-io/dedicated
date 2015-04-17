package collab

import (
	"sync"

	"github.com/nitrous-io/ot.go/ot/session"
)

var (
	Sessions     map[string]*Session
	sessionsLock sync.Mutex
)

func init() {
	Sessions = map[string]*Session{}
}

type Session struct {
	ID          string
	Connections map[string]*Connection
	Lock        sync.Mutex

	*session.Session
}

func GetSession(id string) *Session {
	return Sessions[id]
}

func NewSession(id string, document string) *Session {
	sessionsLock.Lock()
	defer sessionsLock.Unlock()

	if s := Sessions[id]; s != nil {
		return s
	}

	s := &Session{
		ID:          id,
		Session:     session.New(document),
		Connections: map[string]*Connection{},
	}
	Sessions[id] = s
	return s
}

func (s *Session) RegisterConnection(c *Connection) {
	if c.ID == "" {
		return
	}
	s.Lock.Lock()
	s.Connections[c.ID] = c
	s.AddClient(c.ID)
	s.SetName(c.ID, c.Username)
	s.Lock.Unlock()
}

func (s *Session) UnregisterConnection(c *Connection) {
	s.Lock.Lock()
	delete(s.Connections, c.ID)
	s.RemoveClient(c.ID)
	s.Lock.Unlock()

	// close session asychronously
	// if it's done synchronously, it will result in a deadlock
	// because UnregisterConnectionGlobally also locks sessionsLock
	go func() {
		sessionsLock.Lock()
		s.Lock.Lock()
		if len(s.Connections) == 0 {
			delete(Sessions, s.ID)
		}
		s.Lock.Unlock()
		sessionsLock.Unlock()
	}()
}

func UnregisterConnectionGlobally(c *Connection) {
	sessionsLock.Lock()
	for _, s := range Sessions {
		s.UnregisterConnection(c)
	}
	sessionsLock.Unlock()
}

// goes though all registered connections, and updates
// connections with same conn ID with the new conn
func UpdateConnection(c *Connection) {
	if c.ID == "" {
		return
	}
	sessionsLock.Lock()
	for _, s := range Sessions {
		s.Lock.Lock()
		if _, ok := s.Connections[c.ID]; ok {
			s.Connections[c.ID] = c
		}
		s.Lock.Unlock()
	}
	sessionsLock.Unlock()
}
