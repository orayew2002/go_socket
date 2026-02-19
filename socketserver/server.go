package socketserver

import (
	"log"
	"net/http"
	"sync"

	socketio "github.com/googollee/go-socket.io"
	"github.com/googollee/go-socket.io/engineio"
	"github.com/googollee/go-socket.io/engineio/transport"
	"github.com/googollee/go-socket.io/engineio/transport/polling"
	"github.com/googollee/go-socket.io/engineio/transport/websocket"
)

// OTPEvent matches the shape emitted to Socket.IO clients.
type OTPEvent struct {
	Phone string `json:"phone"`
	Pass  string `json:"pass"`
}

type client struct {
	id   string
	busy bool
}

// Manager holds the Socket.IO server and tracks connected clients.
type Manager struct {
	mu      sync.Mutex
	clients map[string]*client
	Server  *socketio.Server
}

// NewManager creates and configures a Socket.IO server.
// Origins are validated upstream by the CORS middleware; here we
// trust all connections that reach this handler.
func NewManager(allowedOrigins []string) *Manager {
	m := &Manager{
		clients: make(map[string]*client),
	}

	originSet := make(map[string]struct{}, len(allowedOrigins))
	for _, o := range allowedOrigins {
		originSet[o] = struct{}{}
	}

	checkOrigin := func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		if origin == "" {
			return true
		}
		_, ok := originSet[origin]
		return ok
	}

	srv := socketio.NewServer(&engineio.Options{
		Transports: []transport.Transport{
			&polling.Transport{
				CheckOrigin: checkOrigin,
			},
			&websocket.Transport{
				CheckOrigin: checkOrigin,
			},
		},
	})

	srv.OnConnect("/", func(s socketio.Conn) error {
		m.mu.Lock()
		m.clients[s.ID()] = &client{id: s.ID(), busy: false}
		count := len(m.clients)
		m.mu.Unlock()
		log.Printf("[SOCKET] Client connected | id=%s | remote=%s | total_clients=%d",
			s.ID(), s.RemoteAddr(), count)
		return nil
	})

	srv.OnError("/", func(s socketio.Conn, err error) {
		log.Printf("[SOCKET] Error | id=%s | remote=%s | error=%v",
			s.ID(), s.RemoteAddr(), err)
	})

	srv.OnEvent("/", "otpsender", func(s socketio.Conn, data interface{}) {
		log.Printf("[SOCKET] Event 'otpsender' received | id=%s | remote=%s | data=%v",
			s.ID(), s.RemoteAddr(), data)
	})

	srv.OnEvent("/", "message", func(s socketio.Conn, data interface{}) {
		log.Printf("[SOCKET] Event 'message' received | id=%s | remote=%s | data=%v",
			s.ID(), s.RemoteAddr(), data)
	})

	srv.OnEvent("/", "sended", func(s socketio.Conn, data interface{}) {
		m.mu.Lock()
		c, ok := m.clients[s.ID()]
		if ok {
			c.busy = false
		}
		m.mu.Unlock()
		if ok {
			log.Printf("[SOCKET] Event 'sended' received, client marked available | id=%s | remote=%s | data=%v",
				s.ID(), s.RemoteAddr(), data)
		} else {
			log.Printf("[SOCKET] Event 'sended' from unknown client | id=%s | remote=%s | data=%v",
				s.ID(), s.RemoteAddr(), data)
		}
	})

	srv.OnDisconnect("/", func(s socketio.Conn, reason string) {
		m.mu.Lock()
		delete(m.clients, s.ID())
		count := len(m.clients)
		m.mu.Unlock()
		log.Printf("[SOCKET] Client disconnected | id=%s | remote=%s | reason=%s | total_clients=%d",
			s.ID(), s.RemoteAddr(), reason, count)
	})

	m.Server = srv
	return m
}

// Emit broadcasts an event to all connected Socket.IO clients.
func (m *Manager) Emit(event string, data interface{}) {
	m.mu.Lock()
	count := len(m.clients)
	m.mu.Unlock()
	log.Printf("[SOCKET] Broadcasting event | event=%s | connected_clients=%d | data=%v", event, count, data)
	m.Server.BroadcastToNamespace("/", event, data)
}
