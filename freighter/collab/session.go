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
	Connections map[*Connection]struct{}
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
		Connections: map[*Connection]struct{}{},
	}
	Sessions[id] = s
	return s
}

func (s *Session) RegisterConnection(c *Connection) {
	s.Lock.Lock()
	s.Connections[c] = struct{}{}
	s.AddClient(c.ID)
	s.SetName(c.ID, c.Username)
	s.Lock.Unlock()
}

func (s *Session) UnregisterConnection(c *Connection) {
	s.Lock.Lock()
	delete(s.Connections, c)
	s.RemoveClient(c.ID)
	// close session asychronously
	// if it's done synchronously, it will result in a deadlock
	// because UnregisterConnectionGlobally also locks sessionsLock
	go func() {
		sessionsLock.Lock()
		if len(s.Connections) == 0 {
			delete(Sessions, s.ID)
		}
		sessionsLock.Unlock()
	}()
	s.Lock.Unlock()
}

func UnregisterConnectionGlobally(c *Connection) {
	sessionsLock.Lock()
	for _, s := range Sessions {
		s.UnregisterConnection(c)
	}
	sessionsLock.Unlock()
}
