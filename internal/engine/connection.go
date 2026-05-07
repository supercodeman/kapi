package engine

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

var allowedOrigins = map[string]bool{
	"http://localhost:3000":  true,
	"http://localhost:3001":  true,
	"http://localhost:8080":  true,
	"http://127.0.0.1:3000": true,
	"http://127.0.0.1:3001": true,
	"http://127.0.0.1:8080": true,
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		if origin == "" {
			return true
		}
		return allowedOrigins[origin]
	},
}

type WSMessage struct {
	Type        string `json:"type"`
	Message     string `json:"message,omitempty"`
	PageContext string `json:"page_context,omitempty"`
	SessionID   uint64 `json:"session_id,omitempty"`
}

type WSResponse struct {
	Type     string      `json:"type"`
	Content  string      `json:"content,omitempty"`
	Data     interface{} `json:"data,omitempty"`
	Done     bool        `json:"done,omitempty"`
	Step     int         `json:"step,omitempty"`
	Total    int         `json:"total,omitempty"`
}

type Connection struct {
	UserID    uint64
	Conn      *websocket.Conn
	Send      chan []byte
	mu        sync.Mutex
}

type ConnectionManager struct {
	connections map[uint64]*Connection
	mu          sync.RWMutex
	engine      *Engine
}

func NewConnectionManager(engine *Engine) *ConnectionManager {
	return &ConnectionManager{
		connections: make(map[uint64]*Connection),
		engine:      engine,
	}
}

func (cm *ConnectionManager) HandleWebSocket(c *gin.Context) {
	userID := c.GetUint64("user_id")
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("websocket upgrade failed: %v", err)
		return
	}

	wsConn := &Connection{
		UserID: userID,
		Conn:   conn,
		Send:   make(chan []byte, 256),
	}

	cm.register(wsConn)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("writePump panic for user %d: %v", wsConn.UserID, r)
			}
		}()
		cm.writePump(wsConn)
	}()
	go cm.readPump(wsConn)
}

func (cm *ConnectionManager) register(conn *Connection) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if old, ok := cm.connections[conn.UserID]; ok {
		close(old.Send)
		old.Conn.Close()
	}
	cm.connections[conn.UserID] = conn
	log.Printf("ws connected: user %d", conn.UserID)
}

func (cm *ConnectionManager) unregister(conn *Connection) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if c, ok := cm.connections[conn.UserID]; ok && c == conn {
		delete(cm.connections, conn.UserID)
		close(conn.Send)
		log.Printf("ws disconnected: user %d", conn.UserID)
	}
}

func (cm *ConnectionManager) readPump(conn *Connection) {
	defer func() {
		cm.unregister(conn)
		conn.Conn.Close()
	}()

	conn.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	conn.Conn.SetPongHandler(func(string) error {
		conn.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		_, message, err := conn.Conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				log.Printf("ws read error: %v", err)
			}
			break
		}

		var wsMsg WSMessage
		if err := json.Unmarshal(message, &wsMsg); err != nil {
			cm.sendError(conn, "invalid message format")
			continue
		}

		switch wsMsg.Type {
		case "chat":
			go func() {
				defer func() {
					if r := recover(); r != nil {
						log.Printf("handleChat panic for user %d: %v", conn.UserID, r)
						cm.sendError(conn, "internal error")
					}
				}()
				cm.handleChat(conn, &wsMsg)
			}()
		case "page_switch":
			log.Printf("user %d switched to page: %s", conn.UserID, wsMsg.PageContext)
		default:
			cm.sendError(conn, "unknown message type")
		}
	}
}

func (cm *ConnectionManager) writePump(conn *Connection) {
	ticker := time.NewTicker(15 * time.Second)
	defer func() {
		ticker.Stop()
		conn.Conn.Close()
	}()

	for {
		select {
		case message, ok := <-conn.Send:
			if !ok {
				conn.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			conn.mu.Lock()
			conn.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := conn.Conn.WriteMessage(websocket.TextMessage, message); err != nil {
				conn.mu.Unlock()
				log.Printf("ws write error for user %d: %v", conn.UserID, err)
				return
			}
			conn.mu.Unlock()
		case <-ticker.C:
			conn.mu.Lock()
			conn.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			err := conn.Conn.WriteMessage(websocket.PingMessage, nil)
			conn.mu.Unlock()
			if err != nil {
				return
			}
		}
	}
}

func (cm *ConnectionManager) handleChat(conn *Connection, msg *WSMessage) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	req := &ChatRequest{
		UserID:      conn.UserID,
		SessionID:   msg.SessionID,
		Message:     msg.Message,
		PageContext:  msg.PageContext,
	}

	resp, err := cm.engine.Chat(ctx, req)
	if err != nil {
		log.Printf("chat error for user %d: %v", conn.UserID, err)
		cm.sendError(conn, "chat failed")
		return
	}

	cm.sendJSON(conn, &WSResponse{
		Type:    "chat_response",
		Content: resp.Display,
		Data:    resp.DataRef,
		Done:    true,
	})
}

func (cm *ConnectionManager) sendJSON(conn *Connection, resp *WSResponse) {
	data, err := json.Marshal(resp)
	if err != nil {
		return
	}
	select {
	case conn.Send <- data:
	default:
		log.Printf("ws send buffer full for user %d", conn.UserID)
	}
}

func (cm *ConnectionManager) sendError(conn *Connection, msg string) {
	cm.sendJSON(conn, &WSResponse{
		Type:    "error",
		Content: msg,
		Done:    true,
	})
}

func (cm *ConnectionManager) SendToUser(userID uint64, resp *WSResponse) bool {
	cm.mu.RLock()
	conn, ok := cm.connections[userID]
	cm.mu.RUnlock()
	if !ok {
		return false
	}
	cm.sendJSON(conn, resp)
	return true
}
