package tokenpony

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

var assetURLPattern = regexp.MustCompile(`^asset://[A-Za-z0-9_-]+$`)

var persistTaskInputRemoteMedia = service.PersistTaskInputRemoteMedia

type requestPayload struct {
	Model           string                      `json:"model"`
	Prompt          string                      `json:"prompt"`
	Resolution      string                      `json:"resolution,omitempty"`
	Ratio           string                      `json:"ratio,omitempty"`
	Duration        *dto.IntValue               `json:"duration,omitempty"`
	Media           []relaycommon.TaskMediaItem `json:"media,omitempty"`
	GenerateAudio   *dto.BoolValue              `json:"generate_audio,omitempty"`
	Watermark       *dto.BoolValue              `json:"watermark,omitempty"`
	Seed            *dto.IntValue               `json:"seed,omitempty"`
	ReturnLastFrame *dto.BoolValue              `json:"return_last_frame,omitempty"`
	ServiceTier     string                      `json:"service_tier,omitempty"`
}

type responseEnvelope struct {
	Code      int             `json:"code"`
	Message   string          `json:"message"`
	RequestID string          `json:"request_id"`
	Data      json.RawMessage `json:"data"`
}

type createData struct {
	ID        string `json:"id"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
}

type taskResult struct {
	Output json.RawMessage `json:"output"`
	Result json.RawMessage `json:"result"`
	Status string          `json:"status"`
	ExecID string          `json:"exec_id"`
}

type taskUsage struct {
	TotalTokens         int     `json:"total_tokens"`
	CompletionTokens    int     `json:"completion_tokens"`
	Duration            int     `json:"duration"`
	Resolution          string  `json:"resolution"`
	GenerateAudio       bool    `json:"generate_audio"`
	TotalTime           float64 `json:"total_time"`
	ReferenceVideoCount int     `json:"reference_video_count"`
}

type queryData struct {
	TaskID        string          `json:"task_id"`
	TaskStatus    string          `json:"task_status"`
	TaskStatusMsg string          `json:"task_status_msg"`
	TaskType      string          `json:"task_type"`
	Result        taskResult      `json:"result"`
	Usage         *taskUsage      `json:"usage"`
	Detail        json.RawMessage `json:"detail"`
	CreatedAt     string          `json:"created_at"`
	UpdatedAt     string          `json:"updated_at"`
}

type TaskAdaptor struct {
	taskcommon.BaseBilling
	apiKey            string
	baseURL           string
	httpTimeoutSecond int
}

func (a *TaskAdaptor) Init(info *relaycommon.RelayInfo) {
	a.apiKey = info.ApiKey
	a.baseURL = strings.TrimRight(info.ChannelBaseUrl, "/")
	if info.ChannelMeta != nil {
		a.httpTimeoutSecond = info.ChannelMeta.ChannelOtherSettings.TokenPonyHTTPTimeoutSeconds
	}
	if a.httpTimeoutSecond <= 0 {
		a.httpTimeoutSecond = defaultHTTPTimeout
	}
}

func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) *dto.TaskError {
	if taskErr := relaycommon.ValidateBasicTaskRequest(c, info, constant.TaskActionGenerate); taskErr != nil {
		return taskErr
	}
	if !strings.HasPrefix(c.GetHeader("Content-Type"), "multipart/form-data") {
		return nil
	}
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_multipart_form", http.StatusBadRequest)
	}
	form, err := common.ParseMultipartFormReusable(c)
	if err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_multipart_form", http.StatusBadRequest)
	}
	defer form.RemoveAll()
	files := form.File["input_reference"]
	if len(files) == 0 {
		return nil
	}
	if len(files) != 1 {
		return service.TaskErrorWrapperLocal(errors.New("exactly one input_reference file is supported"), "invalid_input_reference", http.StatusBadRequest)
	}
	if info.InputMediaFile == "" {
		info.InputMediaFile, info.InputMediaURL, err = service.PersistTaskInputImage(files[0])
		if err != nil {
			return service.TaskErrorWrapperLocal(err, "invalid_input_reference", http.StatusBadRequest)
		}
	}
	// OpenAI input_reference guides generation; it is not defined as a boundary frame.
	req.Media = append(req.Media, relaycommon.TaskMediaItem{Type: "reference_image", URL: info.InputMediaURL})
	c.Set("task_request", req)
	return nil
}

func (a *TaskAdaptor) BuildRequestURL(_ *relaycommon.RelayInfo) (string, error) {
	return a.baseURL + GenerateEndpoint, nil
}

func (a *TaskAdaptor) BuildRequestHeader(_ *gin.Context, req *http.Request, _ *relaycommon.RelayInfo) error {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.apiKey)
	return nil
}

func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	if info.PriceData.UsePrice || info.PriceData.ModelRatio <= 0 {
		return nil, errors.New("TokenPony usage billing requires a positive model multiplier and no fixed per-call model price")
	}
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return nil, err
	}
	modelName := req.Model
	if info.ChannelMeta != nil && info.IsModelMapped {
		modelName = info.UpstreamModelName
	} else {
		info.UpstreamModelName = modelName
	}
	payload, err := convertRequestPayload(&req, modelName)
	if err != nil {
		return nil, err
	}
	rollbackInputMedia, err := materializeBase64ImageMedia(payload, info)
	if err != nil {
		return nil, err
	}
	keepInputMedia := false
	defer func() {
		if !keepInputMedia {
			rollbackInputMedia()
		}
	}()
	if err := validateRequest(payload); err != nil {
		return nil, err
	}
	if err := a.ensureAssetsActive(c.Request.Context(), info, payload); err != nil {
		return nil, err
	}
	rollbackRemoteMedia, err := materializeRemoteMedia(c.Request.Context(), payload, info, time.Duration(a.httpTimeoutSecond)*time.Second)
	if err != nil {
		return nil, err
	}
	defer func() {
		if !keepInputMedia {
			rollbackRemoteMedia()
		}
	}()
	body, err := common.Marshal(payload)
	if err != nil {
		return nil, err
	}
	keepInputMedia = true
	return bytes.NewReader(body), nil
}

type remoteMediaAudit struct {
	Index int `json:"index"`
	service.TaskInputRemoteAudit
}

func materializeRemoteMedia(ctx context.Context, payload *requestPayload, info *relaycommon.RelayInfo, timeout time.Duration) (func(), error) {
	if len(payload.Media) == 0 {
		return func() {}, nil
	}
	if info == nil || info.TaskRelayInfo == nil {
		return func() {}, errors.New("task relay info is required for remote media inputs")
	}
	start := len(info.InputMediaFiles)
	createdKeys := make([]string, 0)
	rollback := func() {
		for _, relative := range info.InputMediaFiles[start:] {
			service.RemoveTaskInputMedia(relative)
		}
		info.InputMediaFiles = info.InputMediaFiles[:start]
		for _, key := range createdKeys {
			delete(info.InputMediaURLs, key)
		}
	}
	stableURLs := map[string]struct{}{}
	if info.InputMediaURL != "" {
		stableURLs[info.InputMediaURL] = struct{}{}
	}
	for _, value := range info.InputMediaURLs {
		stableURLs[value] = struct{}{}
	}
	audits := make([]remoteMediaAudit, 0, len(payload.Media))
	for i := range payload.Media {
		media := &payload.Media[i]
		if strings.HasPrefix(media.URL, "asset://") {
			continue
		}
		if _, ok := stableURLs[media.URL]; ok {
			continue
		}
		key := fmt.Sprintf("remote:%x", sha256.Sum256([]byte(media.URL)))
		if stable, ok := info.InputMediaURLs[key]; ok {
			media.URL = stable
			continue
		}
		relative, publicURL, audit, err := persistTaskInputRemoteMedia(ctx, media.URL, media.Type, timeout)
		record := remoteMediaAudit{Index: i, TaskInputRemoteAudit: audit}
		audits = append(audits, record)
		if err != nil {
			rollback()
			safeAudit, _ := common.Marshal(record)
			logger.LogWarn(ctx, fmt.Sprintf("TokenPony input media stabilization failed: %s error=%s", safeAudit, err.Error()))
			return func() {}, fmt.Errorf("第 %d 项（%s/%s）%w", i+1, tokenPonyMediaLabel(media.Type), media.Type, err)
		}
		if info.InputMediaURLs == nil {
			info.InputMediaURLs = make(map[string]string)
		}
		info.InputMediaURLs[key] = publicURL
		createdKeys = append(createdKeys, key)
		info.InputMediaFiles = append(info.InputMediaFiles, relative)
		stableURLs[publicURL] = struct{}{}
		media.URL = publicURL
		safeAudit, _ := common.Marshal(record)
		logger.LogInfo(ctx, fmt.Sprintf("TokenPony input media stabilized: %s", safeAudit))
	}
	if len(audits) > 0 {
		safeAudit, err := common.Marshal(audits)
		if err != nil {
			rollback()
			return func() {}, err
		}
		info.InputMediaAudit = string(safeAudit)
	}
	return rollback, nil
}

func tokenPonyMediaLabel(mediaType string) string {
	switch mediaType {
	case "reference_image":
		return "参考图"
	case "first_image":
		return "首帧图"
	case "last_image":
		return "尾帧图"
	case "reference_video":
		return "参考视频"
	case "reference_audio":
		return "参考音频"
	default:
		return "媒体"
	}
}

func (a *TaskAdaptor) ensureAssetsActive(ctx context.Context, info *relaycommon.RelayInfo, payload *requestPayload) error {
	assetIDs := make(map[string]struct{})
	for _, media := range payload.Media {
		if strings.HasPrefix(media.URL, "asset://") {
			assetIDs[strings.TrimPrefix(media.URL, "asset://")] = struct{}{}
		}
	}
	for assetID := range assetIDs {
		body, err := common.Marshal(map[string]string{"Id": assetID})
		if err != nil {
			return err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+AssetsEndpoint+"GetAsset", bytes.NewReader(body))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		req.Header.Set("Authorization", "Bearer "+a.apiKey)
		proxyURL := ""
		if info.ChannelMeta != nil {
			proxyURL = info.ChannelMeta.ChannelSetting.Proxy
		}
		client, err := service.GetHttpClientWithProxy(proxyURL)
		if err != nil {
			return fmt.Errorf("validate asset %s: %w", assetID, err)
		}
		clientCopy := *client
		clientCopy.Timeout = time.Duration(a.httpTimeoutSecond) * time.Second
		resp, err := clientCopy.Do(req)
		if err != nil {
			return fmt.Errorf("validate asset %s: %w", assetID, err)
		}
		responseBody, readErr := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
		_ = resp.Body.Close()
		if readErr != nil {
			return fmt.Errorf("validate asset %s: %w", assetID, readErr)
		}
		var envelope struct {
			Result struct {
				ID     string `json:"Id"`
				Status string `json:"Status"`
				Error  string `json:"Error"`
			} `json:"Result"`
			ResponseMetadata struct {
				Error *struct {
					Code    string `json:"Code"`
					Message string `json:"Message"`
				} `json:"Error"`
			} `json:"ResponseMetadata"`
		}
		if err := common.Unmarshal(responseBody, &envelope); err != nil {
			return fmt.Errorf("validate asset %s: invalid GetAsset response", assetID)
		}
		if envelope.ResponseMetadata.Error != nil {
			return fmt.Errorf("validate asset %s: %s: %s", assetID, envelope.ResponseMetadata.Error.Code, envelope.ResponseMetadata.Error.Message)
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return fmt.Errorf("validate asset %s: GetAsset returned HTTP %d", assetID, resp.StatusCode)
		}
		if envelope.Result.Status != "Active" {
			message := fmt.Sprintf("asset %s is not Active (status %q)", assetID, envelope.Result.Status)
			if envelope.Result.Error != "" {
				message += ": " + envelope.Result.Error
			}
			return errors.New(message)
		}
	}
	return nil
}

func (a *TaskAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	info.UpstreamRequestTimeoutSeconds = a.httpTimeoutSecond
	return channel.DoTaskApiRequest(a, c, info, requestBody)
}

func (a *TaskAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (string, []byte, *dto.TaskError) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", nil, service.TaskErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
	}
	_ = resp.Body.Close()

	envelope, taskErr := parseEnvelope(body)
	if taskErr != nil {
		return "", body, taskErr
	}
	var data createData
	if err := common.Unmarshal(envelope.Data, &data); err != nil {
		return "", body, service.TaskErrorWrapper(err, "unmarshal_response_body_failed", http.StatusBadGateway)
	}
	if strings.TrimSpace(data.ID) == "" {
		return "", body, service.TaskErrorWrapper(errors.New("TokenPony response data.id is empty"), "invalid_response", http.StatusBadGateway)
	}

	video := dto.NewOpenAIVideo()
	video.ID = info.PublicTaskID
	video.TaskID = info.PublicTaskID
	video.CreatedAt = time.Now().Unix()
	video.Model = info.OriginModelName
	c.JSON(http.StatusOK, video)
	return data.ID, body, nil
}

func parseEnvelope(body []byte) (*responseEnvelope, *dto.TaskError) {
	var envelope responseEnvelope
	if err := common.Unmarshal(body, &envelope); err != nil {
		return nil, service.TaskErrorWrapper(err, "unmarshal_response_body_failed", http.StatusBadGateway)
	}
	if envelope.Code == http.StatusOK {
		return &envelope, nil
	}
	status := http.StatusBadGateway
	if envelope.Code == 10108 {
		status = http.StatusBadRequest
	} else if envelope.Code == 10107 {
		status = http.StatusNotFound
	}
	err := service.TaskErrorWrapper(fmt.Errorf("TokenPony business error %d: %s", envelope.Code, envelope.Message), fmt.Sprintf("tokenpony_%d", envelope.Code), status)
	err.LocalError = true
	return nil, err
}

func (a *TaskAdaptor) FetchTask(baseURL, key string, body map[string]any, proxy string) (*http.Response, error) {
	taskID, ok := body["task_id"].(string)
	if !ok || strings.TrimSpace(taskID) == "" {
		return nil, errors.New("invalid task_id")
	}
	endpoint := strings.TrimRight(baseURL, "/") + TaskEndpoint + url.PathEscape(taskID) + "?include_detail=true"
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)
	client, err := service.GetHttpClientWithProxy(proxy)
	if err != nil {
		return nil, err
	}
	clientCopy := *client
	clientCopy.Timeout = time.Duration(a.httpTimeoutSecond) * time.Second
	return clientCopy.Do(req)
}

func (a *TaskAdaptor) ParseTaskResult(body []byte) (*relaycommon.TaskInfo, error) {
	envelope, taskErr := parseEnvelope(body)
	if taskErr != nil {
		return relaycommon.FailTaskInfo(taskErr.Message), nil
	}
	var data queryData
	if err := common.Unmarshal(envelope.Data, &data); err != nil {
		return nil, fmt.Errorf("unmarshal TokenPony task data: %w", err)
	}
	result := &relaycommon.TaskInfo{Code: envelope.Code, TaskID: data.TaskID}
	switch data.TaskStatus {
	case "PENDING":
		result.Status = model.TaskStatusQueued
		result.Progress = taskcommon.ProgressQueued
	case "RUNNING":
		result.Status = model.TaskStatusInProgress
		result.Progress = taskcommon.ProgressInProgress
	case "COMPLETED":
		videoURL, lastFrameURL, err := parseOutput(data.Result)
		if err != nil || videoURL == "" {
			result.Status = model.TaskStatusFailure
			result.Progress = taskcommon.ProgressComplete
			if err != nil {
				result.Reason = err.Error()
			} else {
				result.Reason = "TokenPony completed task has no video URL"
			}
			return result, nil
		}
		if data.Usage == nil || data.Usage.TotalTokens <= 0 {
			return nil, errors.New("TokenPony completed task is missing authoritative usage.total_tokens")
		}
		result.Status = model.TaskStatusSuccess
		result.Progress = taskcommon.ProgressComplete
		result.Url = videoURL
		result.LastFrameURL = lastFrameURL
		result.PersistResult = true
		result.TotalTokens = data.Usage.TotalTokens
		result.CompletionTokens = data.Usage.CompletionTokens
	case "CANCELED":
		result.Status = model.TaskStatusFailure
		result.Progress = taskcommon.ProgressComplete
		result.Reason = "TokenPony task was canceled upstream"
	case "FAILED":
		result.Status = model.TaskStatusFailure
		result.Progress = taskcommon.ProgressComplete
		result.Reason = strings.TrimSpace(data.TaskStatusMsg)
		if detail := failureDetail(data.Detail); detail != "" {
			if result.Reason != "" {
				result.Reason += ": "
			}
			result.Reason += detail
		}
		if result.Reason == "" {
			result.Reason = "TokenPony task failed"
		}
	default:
		return nil, fmt.Errorf("unknown TokenPony task status %q", data.TaskStatus)
	}
	return result, nil
}

func parseOutput(result taskResult) (string, string, error) {
	raw := result.Output
	if len(bytes.TrimSpace(raw)) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		raw = result.Result
	}
	if len(bytes.TrimSpace(raw)) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return "", "", errors.New("TokenPony result is missing output and result")
	}
	var value string
	if err := common.Unmarshal(raw, &value); err == nil {
		return strings.TrimSpace(value), "", nil
	}
	var output struct {
		VideoURL     string `json:"video_url"`
		LastFrameURL string `json:"last_frame_url"`
	}
	if err := common.Unmarshal(raw, &output); err != nil {
		return "", "", fmt.Errorf("invalid TokenPony result output: %w", err)
	}
	return strings.TrimSpace(output.VideoURL), strings.TrimSpace(output.LastFrameURL), nil
}

func failureDetail(raw json.RawMessage) string {
	if len(bytes.TrimSpace(raw)) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return ""
	}
	var detail map[string]any
	if err := common.Unmarshal(raw, &detail); err != nil {
		return ""
	}
	for _, key := range []string{"message", "error", "reason"} {
		if value, ok := detail[key].(string); ok {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func (a *TaskAdaptor) GetModelList() []string { return ModelList }
func (a *TaskAdaptor) GetChannelName() string { return ChannelName }

func (a *TaskAdaptor) AdjustBillingOnComplete(task *model.Task, result *relaycommon.TaskInfo) int {
	if result.TotalTokens <= 0 || task.PrivateData.BillingContext == nil {
		return 0
	}
	billing := task.PrivateData.BillingContext
	if billing.ModelRatio <= 0 || billing.GroupRatio <= 0 {
		return 0
	}
	return int(float64(result.TotalTokens) * billing.ModelRatio * billing.GroupRatio)
}

func (a *TaskAdaptor) ConvertToOpenAIVideo(task *model.Task) ([]byte, error) {
	video := dto.NewOpenAIVideo()
	video.ID = task.TaskID
	video.TaskID = task.TaskID
	video.Status = task.Status.ToVideoStatus()
	video.SetProgressStr(task.Progress)
	video.CreatedAt = task.CreatedAt
	video.CompletedAt = task.UpdatedAt
	video.Model = task.Properties.OriginModelName
	if resultURL := task.GetResultURL(); resultURL != "" {
		video.SetMetadata("url", resultURL)
	}
	if lastFrameURL := task.GetLastFrameURL(); lastFrameURL != "" {
		video.SetMetadata("last_frame_url", lastFrameURL)
	}
	if task.Status == model.TaskStatusFailure || task.Status == model.TaskStatusUnknown {
		code := "upstream_failed"
		if task.PrivateData.SubmissionUnknown {
			code = "submission_result_unknown"
		}
		video.Error = &dto.OpenAIVideoError{Message: task.FailReason, Code: code}
	}
	return common.Marshal(video)
}

func convertRequest(req *relaycommon.TaskSubmitReq, modelName string) (*requestPayload, error) {
	payload, err := convertRequestPayload(req, modelName)
	if err != nil {
		return nil, err
	}
	if err := validateRequest(payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func convertRequestPayload(req *relaycommon.TaskSubmitReq, modelName string) (*requestPayload, error) {
	payload := requestPayload{Model: strings.TrimSpace(modelName), Prompt: strings.TrimSpace(req.Prompt)}
	if err := taskcommon.UnmarshalMetadata(req.Metadata, &payload); err != nil {
		return nil, err
	}
	if payload.Prompt == "" {
		payload.Prompt = strings.TrimSpace(req.Prompt)
	}
	if payload.Duration == nil && req.Duration != 0 {
		value := dto.IntValue(req.Duration)
		payload.Duration = &value
	}
	if payload.Resolution == "" {
		payload.Resolution = strings.TrimSpace(req.Resolution)
	}
	if err := applyOpenAISize(&payload, req.Size); err != nil {
		return nil, err
	}
	payload.Media = append(payload.Media, req.Media...)
	if strings.TrimSpace(req.Image) != "" {
		payload.Media = append(payload.Media, relaycommon.TaskMediaItem{Type: "reference_image", URL: strings.TrimSpace(req.Image)})
	}
	for _, imageURL := range req.Images {
		payload.Media = append(payload.Media, relaycommon.TaskMediaItem{Type: "reference_image", URL: strings.TrimSpace(imageURL)})
	}
	for _, item := range req.Content {
		media, ok, err := contentToMedia(item)
		if err != nil {
			return nil, err
		}
		if ok {
			payload.Media = append(payload.Media, media)
		}
	}
	return &payload, nil
}

func materializeBase64ImageMedia(payload *requestPayload, info *relaycommon.RelayInfo) (func(), error) {
	start := 0
	if info != nil && info.TaskRelayInfo != nil {
		start = len(info.InputMediaFiles)
	}
	createdKeys := make([]string, 0)
	rollback := func() {
		if info == nil || info.TaskRelayInfo == nil {
			return
		}
		for _, relative := range info.InputMediaFiles[start:] {
			service.RemoveTaskInputMedia(relative)
		}
		info.InputMediaFiles = info.InputMediaFiles[:start]
		for _, key := range createdKeys {
			delete(info.InputMediaURLs, key)
		}
	}

	for i := range payload.Media {
		media := &payload.Media[i]
		if media.Type != "reference_image" && media.Type != "first_image" && media.Type != "last_image" {
			continue
		}
		value := strings.TrimSpace(media.URL)
		if !strings.HasPrefix(strings.ToLower(value), "data:") {
			continue
		}
		if info == nil || info.TaskRelayInfo == nil {
			rollback()
			return func() {}, errors.New("task relay info is required for base64 image inputs")
		}
		key := fmt.Sprintf("%x", sha256.Sum256([]byte(value)))
		if publicURL, ok := info.InputMediaURLs[key]; ok {
			media.URL = publicURL
			continue
		}
		relative, publicURL, err := service.PersistTaskInputImageDataURI(value)
		if err != nil {
			rollback()
			return func() {}, fmt.Errorf("media[%d]: %w", i, err)
		}
		if info.InputMediaURLs == nil {
			info.InputMediaURLs = make(map[string]string)
		}
		info.InputMediaURLs[key] = publicURL
		info.InputMediaFiles = append(info.InputMediaFiles, relative)
		createdKeys = append(createdKeys, key)
		media.URL = publicURL
	}
	return rollback, nil
}

func contentToMedia(item relaycommon.TaskContentItem) (relaycommon.TaskMediaItem, bool, error) {
	switch item.Type {
	case "text", "":
		return relaycommon.TaskMediaItem{}, false, nil
	case "image_url":
		if item.ImageURL == nil {
			return relaycommon.TaskMediaItem{}, false, errors.New("image_url content is missing image_url.url")
		}
		mediaType := item.Role
		switch mediaType {
		case "", "reference_image":
			mediaType = "reference_image"
		case "first_frame", "first_image":
			mediaType = "first_image"
		case "last_frame", "last_image":
			mediaType = "last_image"
		default:
			return relaycommon.TaskMediaItem{}, false, fmt.Errorf("invalid image role %q", item.Role)
		}
		return relaycommon.TaskMediaItem{Type: mediaType, URL: strings.TrimSpace(item.ImageURL.URL)}, true, nil
	case "video_url":
		if item.VideoURL == nil {
			return relaycommon.TaskMediaItem{}, false, errors.New("video_url content is missing video_url.url")
		}
		if item.Role != "" && item.Role != "reference_video" {
			return relaycommon.TaskMediaItem{}, false, fmt.Errorf("invalid video role %q", item.Role)
		}
		return relaycommon.TaskMediaItem{Type: "reference_video", URL: strings.TrimSpace(item.VideoURL.URL)}, true, nil
	case "audio_url":
		if item.AudioURL == nil {
			return relaycommon.TaskMediaItem{}, false, errors.New("audio_url content is missing audio_url.url")
		}
		if item.Role != "" && item.Role != "reference_audio" {
			return relaycommon.TaskMediaItem{}, false, fmt.Errorf("invalid audio role %q", item.Role)
		}
		return relaycommon.TaskMediaItem{Type: "reference_audio", URL: strings.TrimSpace(item.AudioURL.URL)}, true, nil
	default:
		return relaycommon.TaskMediaItem{}, false, fmt.Errorf("unsupported content type %q", item.Type)
	}
}

func validateRequest(payload *requestPayload) error {
	if payload.Model != ModelSeedance20 && payload.Model != ModelSeedance25 {
		return fmt.Errorf("unsupported TokenPony model %q", payload.Model)
	}
	if payload.Prompt == "" {
		return errors.New("prompt is required")
	}
	resolution, err := normalizeResolution(payload.Resolution)
	if err != nil {
		return err
	}
	payload.Resolution = resolution
	if payload.Ratio != "" && !isOneOf(payload.Ratio, "16:9", "9:16", "4:3", "3:4", "1:1", "21:9", "adaptive") {
		return fmt.Errorf("invalid ratio %q", payload.Ratio)
	}
	if payload.Duration != nil {
		duration := int(*payload.Duration)
		maxDuration := 15
		if payload.Model == ModelSeedance25 {
			maxDuration = 30
		}
		if duration != -1 && (duration < 4 || duration > maxDuration) {
			return fmt.Errorf("duration for %s must be -1 or between 4 and %d", payload.Model, maxDuration)
		}
	}
	if payload.Model == ModelSeedance25 && payload.ServiceTier != "" {
		return errors.New("service_tier is only supported by Seedance 2.0")
	}

	imageCount, videoCount, audioCount := 0, 0, 0
	hasBoundaryFrame := false
	for i := range payload.Media {
		media := &payload.Media[i]
		media.Type = strings.TrimSpace(media.Type)
		media.URL = strings.TrimSpace(media.URL)
		if err := validateMediaURL(media.URL); err != nil {
			return fmt.Errorf("media[%d]: %w", i, err)
		}
		switch media.Type {
		case "reference_image":
			imageCount++
		case "first_image", "last_image":
			imageCount++
			hasBoundaryFrame = true
		case "reference_video":
			videoCount++
		case "reference_audio":
			audioCount++
		default:
			return fmt.Errorf("media[%d] has unsupported type %q", i, media.Type)
		}
	}
	maxImages, maxVideos, maxAudios := 9, 3, 3
	if payload.Model == ModelSeedance25 {
		maxImages, maxVideos, maxAudios = 30, 10, 10
		if hasBoundaryFrame && payload.Ratio != "adaptive" {
			return errors.New("Seedance 2.5 requires ratio adaptive with first_image or last_image")
		}
	} else if audioCount > 0 && imageCount+videoCount == 0 {
		return errors.New("Seedance 2.0 reference_audio requires at least one image or reference_video")
	}
	if imageCount > maxImages || videoCount > maxVideos || audioCount > maxAudios {
		return fmt.Errorf("too many media items for %s: images %d/%d, videos %d/%d, audios %d/%d", payload.Model, imageCount, maxImages, videoCount, maxVideos, audioCount, maxAudios)
	}
	return nil
}

func normalizeResolution(value string) (string, error) {
	value = strings.ToUpper(strings.TrimSpace(value))
	if value == "" || isOneOf(value, "480P", "720P", "1080P") {
		return value, nil
	}
	return "", fmt.Errorf("invalid resolution %q; expected 480P, 720P, or 1080P", value)
}

func applyOpenAISize(payload *requestPayload, size string) error {
	size = strings.TrimSpace(size)
	if size == "" {
		return nil
	}
	type mappedSize struct{ ratio, resolution string }
	mapping := map[string]mappedSize{
		"1280x720":  {ratio: "16:9", resolution: "720P"},
		"720x1280":  {ratio: "9:16", resolution: "720P"},
		"1920x1080": {ratio: "16:9", resolution: "1080P"},
		"1080x1920": {ratio: "9:16", resolution: "1080P"},
	}
	mapped, ok := mapping[size]
	if !ok {
		return fmt.Errorf("unsupported size %q; expected 1280x720, 720x1280, 1920x1080, or 1080x1920", size)
	}
	resolution, err := normalizeResolution(payload.Resolution)
	if err != nil {
		return err
	}
	if resolution != "" && resolution != mapped.resolution {
		return fmt.Errorf("size %q conflicts with resolution %q", size, payload.Resolution)
	}
	if payload.Ratio != "" && payload.Ratio != mapped.ratio {
		return fmt.Errorf("size %q conflicts with ratio %q", size, payload.Ratio)
	}
	payload.Resolution = mapped.resolution
	payload.Ratio = mapped.ratio
	return nil
}

func validateMediaURL(value string) error {
	if assetURLPattern.MatchString(value) {
		return nil
	}
	u, err := url.Parse(value)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return errors.New("url must be public HTTP(S) or asset://<id>")
	}
	return nil
}

func isOneOf(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}
