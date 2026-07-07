package chatroom

import (
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

const (
	DefaultRoomID        = "default"
	HeartbeatTimeout     = 90 * time.Second
	HeartbeatGracePeriod = 30 * time.Second
	writeWait            = 10 * time.Second
	pongWait             = 60 * time.Second
	pingPeriod           = (pongWait * 9) / 10
	maxMessageSize       = 4096
)

var globalHub = NewHub()

func GetHub() *Hub {
	return globalHub
}

type Hub struct {
	mu    sync.RWMutex
	rooms map[string]*Room
}

type Room struct {
	id      string
	mu      sync.RWMutex
	clients map[string]*Client
}

type Client struct {
	id             string
	roomID         string
	conn           *websocket.Conn
	send           chan []byte
	lastHeartbeat  time.Time
	hub            *Hub
	onceUnregister sync.Once
}

func NewHub() *Hub {
	h := &Hub{
		rooms: make(map[string]*Room),
	}
	go h.cleanupLoop()
	return h
}

func normalizeRoomID(roomID string) string {
	if roomID == "" {
		return DefaultRoomID
	}
	return roomID
}

func (h *Hub) getOrCreateRoom(roomID string) *Room {
	roomID = normalizeRoomID(roomID)
	h.mu.Lock()
	defer h.mu.Unlock()
	if room, ok := h.rooms[roomID]; ok {
		return room
	}
	room := &Room{
		id:      roomID,
		clients: make(map[string]*Client),
	}
	h.rooms[roomID] = room
	return room
}

func (h *Hub) Register(roomID, clientID string, conn *websocket.Conn) (*Client, error) {
	roomID = normalizeRoomID(roomID)
	if clientID == "" {
		clientID = uuid.NewString()
	}

	room := h.getOrCreateRoom(roomID)

	room.mu.Lock()
	if existing, ok := room.clients[clientID]; ok {
		existing.unregister()
	}
	client := &Client{
		id:            clientID,
		roomID:        roomID,
		conn:          conn,
		send:          make(chan []byte, 64),
		lastHeartbeat: time.Now(),
		hub:           h,
	}
	room.clients[clientID] = client
	room.mu.Unlock()

	go client.writePump()
	go client.readPump()

	return client, nil
}

func (h *Hub) TouchClient(roomID, clientID string) (dto.ChatHeartbeatResponse, bool) {
	roomID = normalizeRoomID(roomID)
	room := h.getOrCreateRoom(roomID)

	room.mu.Lock()
	defer room.mu.Unlock()

	client, ok := room.clients[clientID]
	if !ok {
		return dto.ChatHeartbeatResponse{
			ClientID:    clientID,
			RoomID:      roomID,
			Online:      false,
			OnlineCount: len(room.clients),
		}, false
	}

	now := time.Now()
	client.lastHeartbeat = now
	return dto.ChatHeartbeatResponse{
		ClientID:      clientID,
		RoomID:        roomID,
		Online:        true,
		LastHeartbeat: now.Unix(),
		OnlineCount:   len(room.clients),
	}, true
}

func (h *Hub) OnlineCount(roomID string) int {
	roomID = normalizeRoomID(roomID)
	h.mu.RLock()
	room, ok := h.rooms[roomID]
	h.mu.RUnlock()
	if !ok {
		return 0
	}
	room.mu.RLock()
	defer room.mu.RUnlock()
	return len(room.clients)
}

func (h *Hub) Broadcast(roomID string, message dto.ChatMessage) error {
	roomID = normalizeRoomID(roomID)
	payload, err := common.Marshal(message)
	if err != nil {
		return err
	}

	h.mu.RLock()
	room, ok := h.rooms[roomID]
	h.mu.RUnlock()
	if !ok {
		return nil
	}

	room.mu.RLock()
	clients := make([]*Client, 0, len(room.clients))
	for _, client := range room.clients {
		clients = append(clients, client)
	}
	room.mu.RUnlock()

	for _, client := range clients {
		select {
		case client.send <- payload:
		default:
			go client.unregister()
		}
	}
	return nil
}

func (h *Hub) SendMessage(roomID, nickname, content, msgType, clientID string, clientTimestamp *int64) (dto.ChatMessage, error) {
	if msgType == "" {
		msgType = "message"
	}

	timestamp := time.Now().UnixMilli()
	if clientTimestamp != nil && *clientTimestamp > 0 {
		timestamp = *clientTimestamp
	}

	message := dto.ChatMessage{
		ID:        uuid.NewString(),
		RoomID:    normalizeRoomID(roomID),
		Type:      msgType,
		Content:   content,
		Text:      content,
		ClientID:  strings.TrimSpace(clientID),
		Nickname:  nickname,
		Sender:    nickname,
		Timestamp: timestamp,
	}
	if err := h.Broadcast(message.RoomID, message); err != nil {
		return dto.ChatMessage{}, err
	}
	return message, nil
}

func (h *Hub) HandleIncomingWSMessage(client *Client, data []byte) {
	client.lastHeartbeat = time.Now()

	var req dto.ChatSendRequest
	if err := common.Unmarshal(data, &req); err != nil {
		return
	}

	msgType := strings.ToLower(strings.TrimSpace(req.Type))
	switch msgType {
	case "ping":
		client.sendPong()
		return
	case "pong", "heartbeat", "heartbeat_ack":
		return
	case "message", "":
		roomID, nickname, content, normalizedType, clientID, err := normalizeWSMessage(&req, client.roomID)
		if err != nil {
			return
		}
		_, _ = h.SendMessage(roomID, nickname, content, normalizedType, clientID, req.Timestamp)
	}
}

func normalizeWSMessage(req *dto.ChatSendRequest, defaultRoomID string) (roomID, nickname, content, msgType, clientID string, err error) {
	roomID = strings.TrimSpace(req.RoomID)
	if roomID == "" {
		roomID = defaultRoomID
	}

	content = strings.TrimSpace(req.Content)
	if content == "" {
		content = strings.TrimSpace(req.Text)
	}
	if content == "" {
		return "", "", "", "", "", errEmptyMessage
	}

	nickname = strings.TrimSpace(req.Nickname)
	if nickname == "" {
		nickname = strings.TrimSpace(req.Sender)
	}

	msgType = strings.TrimSpace(req.Type)
	if msgType == "" {
		msgType = "message"
	}

	clientID = strings.TrimSpace(req.ClientID)
	if clientID == "" {
		clientID = strings.TrimSpace(req.ClientIDSnake)
	}

	return roomID, nickname, content, msgType, clientID, nil
}

var errEmptyMessage = errChatMessage("content or text is required")

type errChatMessage string

func (e errChatMessage) Error() string {
	return string(e)
}

func (c *Client) sendPong() {
	payload, err := common.Marshal(dto.ChatPongMessage{
		Type:      "pong",
		Timestamp: time.Now().UnixMilli(),
	})
	if err != nil {
		return
	}
	select {
	case c.send <- payload:
	default:
	}
}

func (c *Client) ID() string {
	return c.id
}

func (c *Client) RoomID() string {
	return c.roomID
}

func (c *Client) SendChan() chan []byte {
	return c.send
}

func (c *Client) unregister() {
	c.onceUnregister.Do(func() {
		c.hub.mu.RLock()
		room, ok := c.hub.rooms[c.roomID]
		c.hub.mu.RUnlock()
		if !ok {
			return
		}

		room.mu.Lock()
		if current, exists := room.clients[c.id]; exists && current == c {
			delete(room.clients, c.id)
		}
		room.mu.Unlock()

		_ = c.conn.Close()
		close(c.send)
	})
}

func (c *Client) readPump() {
	defer c.unregister()

	c.conn.SetReadLimit(maxMessageSize)
	_ = c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		_ = c.conn.SetReadDeadline(time.Now().Add(pongWait))
		c.lastHeartbeat = time.Now()
		return nil
	})

	for {
		_, data, err := c.conn.ReadMessage()
		if err != nil {
			break
		}
		c.hub.HandleIncomingWSMessage(c, data)
	}
}

func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.unregister()
	}()

	for {
		select {
		case message, ok := <-c.send:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				_ = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}
		case <-ticker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (h *Hub) cleanupLoop() {
	ticker := time.NewTicker(HeartbeatGracePeriod)
	defer ticker.Stop()

	for range ticker.C {
		now := time.Now()
		h.mu.RLock()
		rooms := make([]*Room, 0, len(h.rooms))
		for _, room := range h.rooms {
			rooms = append(rooms, room)
		}
		h.mu.RUnlock()

		for _, room := range rooms {
			room.mu.Lock()
			stale := make([]*Client, 0)
			for _, client := range room.clients {
				if now.Sub(client.lastHeartbeat) > HeartbeatTimeout {
					stale = append(stale, client)
				}
			}
			room.mu.Unlock()

			for _, client := range stale {
				client.unregister()
			}
		}
	}
}
