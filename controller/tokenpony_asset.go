package controller

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/relay/channel/task/tokenpony"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

func RelayTokenPonyAsset(c *gin.Context) {
	action := c.Param("action")
	if !tokenpony.IsAssetActionAllowed(action) {
		c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"code": "unsupported_asset_action", "message": "Only the 10 documented TokenPony asset actions are supported"}})
		return
	}
	if common.GetContextKeyInt(c, constant.ContextKeyChannelType) != constant.ChannelTypeTokenPony {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "invalid_channel_type", "message": "Asset requests require a TokenPony channel"}})
		return
	}
	if tokenpony.IsLivenessAction(action) && !strings.EqualFold(c.GetHeader("X-New-Api-Liveness-Confirmed"), "true") {
		c.JSON(http.StatusConflict, gin.H{"error": gin.H{"code": "liveness_confirmation_required", "message": "Explicit liveness confirmation is required for this action"}})
		return
	}

	var payload map[string]any
	if err := common.UnmarshalBodyReusable(c, &payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "invalid_request", "message": err.Error()}})
		return
	}
	delete(payload, "model")
	delete(payload, "group")
	delete(payload, "confirm_liveness")
	body, err := common.Marshal(payload)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "invalid_request", "message": err.Error()}})
		return
	}

	baseURL := strings.TrimRight(common.GetContextKeyString(c, constant.ContextKeyChannelBaseUrl), "/")
	req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodPost, baseURL+tokenpony.AssetsEndpoint+action, bytes.NewReader(body))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "request_build_failed", "message": err.Error()}})
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+common.GetContextKeyString(c, constant.ContextKeyChannelKey))

	channelSetting, _ := common.GetContextKeyType[dto.ChannelSettings](c, constant.ContextKeyChannelSetting)
	otherSetting, _ := common.GetContextKeyType[dto.ChannelOtherSettings](c, constant.ContextKeyChannelOtherSetting)
	client, err := service.GetHttpClientWithProxy(channelSetting.Proxy)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": gin.H{"code": "proxy_client_failed", "message": err.Error()}})
		return
	}
	clientCopy := *client
	timeout := otherSetting.TokenPonyHTTPTimeoutSeconds
	if timeout <= 0 {
		timeout = 30
	}
	clientCopy.Timeout = time.Duration(timeout) * time.Second
	resp, err := clientCopy.Do(req)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": gin.H{"code": "upstream_request_failed", "message": "TokenPony asset request failed"}})
		return
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, 8*1024*1024))
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": gin.H{"code": "upstream_response_failed", "message": err.Error()}})
		return
	}

	var envelope map[string]any
	if err := common.Unmarshal(responseBody, &envelope); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": gin.H{"code": "invalid_upstream_response", "message": "TokenPony asset response is not valid JSON"}})
		return
	}
	_, hasResult := envelope["Result"]
	metadata, _ := envelope["ResponseMetadata"].(map[string]any)
	_, hasError := metadata["Error"]
	status := resp.StatusCode
	if status >= 200 && status < 300 && !hasResult && hasError {
		status = http.StatusBadRequest
	}
	if !hasResult && !hasError {
		c.JSON(http.StatusBadGateway, gin.H{"error": gin.H{"code": "invalid_upstream_response", "message": fmt.Sprintf("TokenPony %s response has neither Result nor ResponseMetadata.Error", action)}})
		return
	}
	c.Data(status, "application/json", responseBody)
}
