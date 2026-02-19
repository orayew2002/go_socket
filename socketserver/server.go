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
// All origins are allowed.
func NewManager() *Manager {
	m := &Manager{
		clients: make(map[string]*client),
	}

	allowAll := func(r *http.Request) bool { return true }

	srv := socketio.NewServer(&engineio.Options{
		Transports: []transport.Transport{
			&polling.Transport{
				CheckOrigin: allowAll,
			},
			&websocket.Transport{
				CheckOrigin: allowAll,
			},
		},
	})

	// go-socket.io v1.7.0 fires OnConnect twice for the same connection when
	// the client upgrades from polling → WebSocket transport. Guard with a
	// duplicate check so the client map and counter stay correct.
	srv.OnConnect("/", func(s socketio.Conn) error {
		m.mu.Lock()
		if _, exists := m.clients[s.ID()]; exists {
			m.mu.Unlock()
			log.Printf("[SOCKET] Duplicate OnConnect (transport upgrade) – ignored | id=%s | remote=%s",
				s.ID(), s.RemoteAddr())
			return nil
		}
		m.clients[s.ID()] = &client{id: s.ID(), busy: false}
		count := len(m.clients)
		m.mu.Unlock()
		log.Printf("[SOCKET] Client connected | id=%s | remote=%s | total_clients=%d",
			s.ID(), s.RemoteAddr(), count)
		return nil
	})

	// OnError is called when a connection error occurs (e.g. i/o timeout after
	// a client drops silently). In go-socket.io v1.7.0, `s` can be nil for
	// errors that occur before a connection is fully established, so we guard
	// against that to avoid a nil-pointer panic crashing the whole process.
	srv.OnError("/", func(s socketio.Conn, err error) {
		if s == nil {
			log.Printf("[SOCKET] Error (no connection context) | error=%v", err)
			return
		}
		// "i/o timeout" is a normal event – it means the remote peer dropped
		// the TCP connection without sending a close frame. The client will
		// reconnect automatically; no action needed.
		log.Printf("[SOCKET] Connection error | id=%s | remote=%s | error=%v",
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
			log.Printf("[SOCKET] Event 'sended' – client marked available | id=%s | remote=%s | data=%v",
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
