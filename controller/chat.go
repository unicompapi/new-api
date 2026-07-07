package controller

import (
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/service/chatroom"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

var chatUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func ChatReceive(c *gin.Context) {
	roomID := strings.TrimSpace(c.Query("room_id"))
	clientID := strings.TrimSpace(c.Query("client_id"))

	conn, err := chatUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		common.SysLog("chat websocket upgrade failed: " + err.Error())
		return
	}

	_, err = chatroom.GetHub().Register(roomID, clientID, conn)
	if err != nil {
		_ = conn.Close()
		common.SysLog("chat websocket register failed: " + err.Error())
	}
}

func ChatSend(c *gin.Context) {
	var req dto.ChatSendRequest
	if err := common.UnmarshalBodyReusable(c, &req); err != nil {
		common.ApiError(c, err)
		return
	}

	roomID, nickname, content, msgType, err := normalizeChatSendRequest(&req)
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}

	message, err := chatroom.GetHub().SendMessage(
		roomID,
		nickname,
		content,
		msgType,
		resolveClientID(req.ClientID, req.ClientIDSnake),
		req.Timestamp,
	)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	common.ApiSuccess(c, message)
}

func ChatHeartbeat(c *gin.Context) {
	if c.Request.Method == http.MethodGet {
		common.ApiSuccess(c, dto.ChatHeartbeatResponse{Status: "ok"})
		return
	}

	var req dto.ChatHeartbeatRequest
	if err := common.UnmarshalBodyReusable(c, &req); err != nil {
		common.ApiError(c, err)
		return
	}

	req.RoomID = strings.TrimSpace(req.RoomID)
	req.ClientID = strings.TrimSpace(req.ClientID)

	if req.ClientID == "" {
		common.ApiSuccess(c, dto.ChatHeartbeatResponse{Status: "ok"})
		return
	}

	resp, online := chatroom.GetHub().TouchClient(req.RoomID, req.ClientID)
	if !online {
		common.ApiErrorMsg(c, "client is not online, please connect to /ws/receive first")
		return
	}

	resp.Status = "ok"
	common.ApiSuccess(c, resp)
}

func normalizeChatSendRequest(req *dto.ChatSendRequest) (roomID, nickname, content, msgType string, err error) {
	roomID = strings.TrimSpace(req.RoomID)

	content = strings.TrimSpace(req.Content)
	if content == "" {
		content = strings.TrimSpace(req.Text)
	}

	nickname = strings.TrimSpace(req.Nickname)
	if nickname == "" {
		nickname = strings.TrimSpace(req.Sender)
	}

	msgType = strings.TrimSpace(req.Type)
	if msgType == "" {
		msgType = "message"
	}

	if content == "" {
		return "", "", "", "", errRequired("content or text is required")
	}

	return roomID, nickname, content, msgType, nil
}

func resolveClientID(clientID, clientIDSnake string) string {
	if id := strings.TrimSpace(clientID); id != "" {
		return id
	}
	return strings.TrimSpace(clientIDSnake)
}

type requiredFieldError string

func (e requiredFieldError) Error() string {
	return string(e)
}

func errRequired(msg string) error {
	return requiredFieldError(msg)
}
