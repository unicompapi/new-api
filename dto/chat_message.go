package dto

type ChatSendRequest struct {
	RoomID        string `json:"room_id"`
	Type          string `json:"type"`
	Content       string `json:"content"`
	Text          string `json:"text"`
	Nickname      string `json:"nickname"`
	Sender        string `json:"sender"`
	ClientID      string `json:"clientId"`
	ClientIDSnake string `json:"client_id"`
	Timestamp     *int64 `json:"timestamp"`
}

type ChatHeartbeatRequest struct {
	RoomID   string `json:"room_id"`
	ClientID string `json:"client_id"`
}

type ChatMessage struct {
	ID        string `json:"id,omitempty"`
	RoomID    string `json:"room_id,omitempty"`
	Type      string `json:"type"`
	Content   string `json:"content"`
	Text      string `json:"text"`
	ClientID  string `json:"clientId,omitempty"`
	Nickname  string `json:"nickname,omitempty"`
	Sender    string `json:"sender,omitempty"`
	Timestamp int64  `json:"timestamp"`
}

type ChatHeartbeatResponse struct {
	Status        string `json:"status,omitempty"`
	ClientID      string `json:"client_id,omitempty"`
	RoomID        string `json:"room_id,omitempty"`
	Online        bool   `json:"online,omitempty"`
	LastHeartbeat int64  `json:"last_heartbeat,omitempty"`
	OnlineCount   int    `json:"online_count,omitempty"`
}

type ChatPongMessage struct {
	Type      string `json:"type"`
	Timestamp int64  `json:"timestamp"`
}
