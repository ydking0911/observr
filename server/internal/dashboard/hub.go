// Package dashboard serves the React UI and streams events over WebSocket.
package dashboard

import (
	"embed"
	"encoding/json"
	"io/fs"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/your-org/observr/server/internal/storage"
)

//go:embed dist/*
var embeddedUI embed.FS

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 4096,
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		return origin == "" ||
			origin == "http://localhost:7676" ||
			origin == "http://localhost:5173" // Vite dev server
	},
}

// Hub manages WebSocket clients and broadcasts new events.
type Hub struct {
	store   querier
	mu      sync.RWMutex
	clients map[*client]struct{}
	join    chan *client
	leave   chan *client
	outbox  chan []byte
}

type querier interface {
	Query(f storage.QueryFilter) ([]storage.Event, error)
}

type client struct {
	conn *websocket.Conn
	send chan []byte
	hub  *Hub
}

func NewHub(s querier) *Hub {
	return &Hub{
		store:   s,
		clients: make(map[*client]struct{}),
		join:    make(chan *client, 32),
		leave:   make(chan *client, 32),
		outbox:  make(chan []byte, 1024),
	}
}

// Broadcast implements storage.Broadcaster — called by the store on Insert.
func (h *Hub) Broadcast(event storage.Event) {
	data, err := json.Marshal(event)
	if err != nil {
		return
	}
	select {
	case h.outbox <- data:
	default:
		// hub overloaded — drop silently
	}
}

// Run processes join/leave/broadcast. Must run in its own goroutine.
func (h *Hub) Run() {
	for {
		select {
		case c := <-h.join:
			h.mu.Lock()
			h.clients[c] = struct{}{}
			h.mu.Unlock()

		case c := <-h.leave:
			h.mu.Lock()
			if _, ok := h.clients[c]; ok {
				delete(h.clients, c)
				close(c.send)
			}
			h.mu.Unlock()

		case data := <-h.outbox:
			h.mu.RLock()
			for c := range h.clients {
				select {
				case c.send <- data:
				default:
					// slow client — schedule disconnect
					go func(cl *client) { h.leave <- cl }(c)
				}
			}
			h.mu.RUnlock()
		}
	}
}

// ServeHTTP handles WebSocket upgrade at GET /ws.
func (h *Hub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("ws upgrade: %v", err)
		return
	}

	c := &client{conn: conn, send: make(chan []byte, 256), hub: h}
	h.join <- c

	// Replay last 50 events on connect so the dashboard loads instantly
	if events, err := h.store.Query(storage.QueryFilter{Last: 50}); err == nil {
		for i := len(events) - 1; i >= 0; i-- {
			if data, err := json.Marshal(events[i]); err == nil {
				c.send <- data
			}
		}
	}

	go c.writePump()
	go c.readPump()
}

// ── Client write/read pumps ───────────────────────────────────────────────

const (
	writeWait  = 10 * time.Second
	pongWait   = 60 * time.Second
	pingPeriod = (pongWait * 9) / 10
)

func (c *client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case msg, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, nil) //nolint:errcheck
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (c *client) readPump() {
	defer func() {
		c.hub.leave <- c
		c.conn.Close()
	}()
	c.conn.SetReadLimit(512)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})
	// Drain — dashboard only sends pong frames
	for {
		if _, _, err := c.conn.ReadMessage(); err != nil {
			return
		}
	}
}

// ── Static dashboard ──────────────────────────────────────────────────────

// StaticHandler serves the compiled React dashboard embedded in the binary.
// Falls back to a helpful placeholder when dist/ isn't built yet.
func StaticHandler() http.Handler {
	sub, err := fs.Sub(embeddedUI, "dist")
	if err != nil {
		return http.HandlerFunc(placeholderHandler)
	}
	return http.FileServer(http.FS(sub))
}

func placeholderHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(placeholderHTML)) //nolint:errcheck
}

const placeholderHTML = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8" />
  <title>observr</title>
  <style>
    body { font-family: monospace; background: #0d1117; color: #e6edf3;
           padding: 2rem; max-width: 600px; margin: 0 auto; }
    h1   { color: #58a6ff; }
    p    { color: #8b949e; line-height: 1.7; }
    pre  { background: #161b22; padding: 1rem; border-radius: 6px; overflow-x: auto; }
    a    { color: #58a6ff; }
    code { background: #161b22; padding: .15em .4em; border-radius: 3px; color: #79c0ff; }
  </style>
</head>
<body>
  <h1>observr</h1>
  <p>Collector is running on this port. Build the dashboard to see the UI:</p>
  <pre>cd dashboard && npm install && npm run build</pre>
  <p>Or query events directly:
    <a href="/query?format=json&last=20">/query?format=json&amp;last=20</a>
  </p>
</body>
</html>`
