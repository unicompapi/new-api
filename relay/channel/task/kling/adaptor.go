package kling

import (
	"bytes"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"github.com/samber/lo"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/pkg/errors"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/relay/channel"
	taskcommon "github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
)

// klingResolution1080PRatio is the price multiplier for 1080P vs 720P for kling-v3 models.
// 720P: ¥0.8/s (baseline), 1080P: ¥1.0/s → ratio = 1.0/0.8 = 1.25
const klingResolution1080PRatio = 1.0 / 0.8

// isKlingCNYModel reports whether the model uses per-second CNY pricing.
func isKlingCNYModel(model string) bool {
	return strings.HasPrefix(strings.ToLower(model), "kling-v3")
}

// ============================
// Request / Response structures
// ============================

type TrajectoryPoint struct {
	X int `json:"x"`
	Y int `json:"y"`
}

type DynamicMask struct {
	Mask         string            `json:"mask,omitempty"`
	Trajectories []TrajectoryPoint `json:"trajectories,omitempty"`
}

type CameraConfig struct {
	Horizontal float64 `json:"horizontal,omitempty"`
	Vertical   float64 `json:"vertical,omitempty"`
	Pan        float64 `json:"pan,omitempty"`
	Tilt       float64 `json:"tilt,omitempty"`
	Roll       float64 `json:"roll,omitempty"`
	Zoom       float64 `json:"zoom,omitempty"`
}

type CameraControl struct {
	Type   string        `json:"type,omitempty"`
	Config *CameraConfig `json:"config,omitempty"`
}

// requestPayload is the upstream request body for Kling v1/v2 models.
type requestPayload struct {
	Prompt         string         `json:"prompt,omitempty"`
	Image          string         `json:"image,omitempty"`
	ImageTail      string         `json:"image_tail,omitempty"`
	NegativePrompt string         `json:"negative_prompt,omitempty"`
	Mode           string         `json:"mode,omitempty"`
	Duration       string         `json:"duration,omitempty"` // string for v1/v2
	AspectRatio    string         `json:"aspect_ratio,omitempty"`
	ModelName      string         `json:"model_name,omitempty"`
	Model          string         `json:"model,omitempty"` // Compatible with upstreams that only recognize "model"
	CfgScale       float64        `json:"cfg_scale,omitempty"`
	StaticMask     string         `json:"static_mask,omitempty"`
	DynamicMasks   []DynamicMask  `json:"dynamic_masks,omitempty"`
	CameraControl  *CameraControl `json:"camera_control,omitempty"`
	CallbackUrl    string         `json:"callback_url,omitempty"`
	ExternalTaskId string         `json:"external_task_id,omitempty"`
}

// ----------------------------------------------------------------
// v3 request structs (kling-3.0-turbo, etc.)
//
// Endpoint (T2V): POST /text-to-video/{api-model-id}
// Endpoint (I2V): POST /image-to-video/{api-model-id}
//
// The model identifier is embedded in the URL, NOT in the body.
// ----------------------------------------------------------------

// v3WatermarkInfo controls watermark on the generated video.
type v3WatermarkInfo struct {
	Enabled bool `json:"enabled"`
}

// v3Options holds delivery-related options for a v3 request.
type v3Options struct {
	CallbackUrl    string           `json:"callback_url,omitempty"`
	WatermarkInfo  *v3WatermarkInfo `json:"watermark_info,omitempty"`
	ExternalTaskId string           `json:"external_task_id,omitempty"`
}

// v3Settings holds generation parameters for a v3 request.
type v3Settings struct {
	Duration    int    `json:"duration,omitempty"`    // 3–15 s (default 5)
	Resolution  string `json:"resolution,omitempty"`  // "720p" | "1080p"
	AspectRatio string `json:"aspect_ratio,omitempty"` // "16:9" | "9:16" | "1:1" (T2V only)
}

// v3Content is one item in the contents array used by the I2V endpoint.
//
//	type "prompt"      → { text }           text prompt
//	type "first_frame" → { url }            first-frame image URL
//	type "last_frame"  → { url }            last-frame image URL
type v3Content struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
	Url  string `json:"url,omitempty"`
}

// v3RequestPayload is the upstream request body for Kling v3.
//
// T2V (POST /text-to-video/{model}):
//
//	{ "prompt": "...", "settings": {...}, "options": {...} }
//
// I2V (POST /image-to-video/{model}):
//
//	{ "contents": [{type:"prompt",text:"..."},{type:"first_frame",url:"..."}],
//	  "settings": {...}, "options": {...} }
//
// Note: for I2V the top-level prompt is absent; settings.aspect_ratio is omitted.
type v3RequestPayload struct {
	// T2V: top-level text prompt
	Prompt string `json:"prompt,omitempty"`
	// I2V: contents array (prompt + first_frame items)
	Contents []v3Content `json:"contents,omitempty"`
	Options  *v3Options  `json:"options,omitempty"`
	Settings *v3Settings `json:"settings,omitempty"`
}

// ----------------------------------------------------------------
// v1/v2 response struct
// ----------------------------------------------------------------

// responsePayload is the upstream response for v1/v2 Kling models.
// data is an object: data.task_id / data.task_status / "succeed"
type responsePayload struct {
	Code      int    `json:"code"`
	Message   string `json:"message"`
	RequestId string `json:"request_id"`
	TaskId    string `json:"task_id"` // v1/v2 top-level fallback
	Data      struct {
		TaskId        string `json:"task_id"`
		TaskStatus    string `json:"task_status"`
		TaskStatusMsg string `json:"task_status_msg"`
		TaskInfo      struct {
			ExternalTaskId string `json:"external_task_id"`
		} `json:"task_info"`
		TaskResult struct {
			Videos []struct {
				Id           string `json:"id"`
				Url          string `json:"url"`
				WatermarkUrl string `json:"watermark_url"`
				Duration     string `json:"duration"`
			} `json:"videos"`
			Images []struct {
				Index        int    `json:"index"`
				Url          string `json:"url"`
				WatermarkUrl string `json:"watermark_url"`
			} `json:"images"`
		} `json:"task_result"`
		CreatedAt          int64  `json:"created_at"`
		UpdatedAt          int64  `json:"updated_at"`
		FinalUnitDeduction string `json:"final_unit_deduction"`
	} `json:"data"`
}

// ----------------------------------------------------------------
// v3 query response structs
//
// Query endpoint: GET /tasks?task_ids={taskID}
// data is an array; outputs are polymorphic by "type" field.
// ----------------------------------------------------------------

// v3Output is one item in the outputs array of a v3 task.
// The "type" field distinguishes video / image / audio / voice / element.
type v3Output struct {
	Type         string `json:"type"`
	Id           string `json:"id"`
	Url          string `json:"url"`
	WatermarkUrl string `json:"watermark_url"`
	Duration     string `json:"duration"`  // video only
	Mp3Url       string `json:"mp3_url"`   // audio only
	WavUrl       string `json:"wav_url"`   // audio only
}

// v3Billing records charge information for a v3 task.
type v3Billing struct {
	ChargeType  string `json:"charge_type"`
	Amount      string `json:"amount"`
	PackageType string `json:"package_type"`
}

// v3TaskItem is one entry in the data array returned by GET /tasks?task_ids=...
type v3TaskItem struct {
	Id         string      `json:"id"`
	Status     string      `json:"status"` // "submitted"|"processing"|"succeeded"|"failed"
	Message    string      `json:"message"`
	CreateTime int64       `json:"create_time"` // ms
	UpdateTime int64       `json:"update_time"` // ms
	ExternalId string      `json:"external_id"`
	Outputs    []v3Output  `json:"outputs"`
	Billing    []v3Billing `json:"billing"`
}

// firstVideoOutput returns the first output with type=="video", or nil.
func (t *v3TaskItem) firstVideoOutput() *v3Output {
	for i := range t.Outputs {
		if t.Outputs[i].Type == "video" {
			return &t.Outputs[i]
		}
	}
	return nil
}

// v3QueryResponse is the full response from GET /tasks?task_ids=...
type v3QueryResponse struct {
	Code      int          `json:"code"`
	Message   string       `json:"message"`
	RequestId string       `json:"request_id"`
	Data      []v3TaskItem `json:"data"`
}

// v3CreateResponse is the response from POST /text-to-video/{model-id} (create task).
// data is a single object (not an array), containing the new task's ID and initial status.
type v3CreateResponse struct {
	Code      int    `json:"code"`
	Message   string `json:"message"`
	RequestId string `json:"request_id"`
	Data      struct {
		Id         string `json:"id"`
		Status     string `json:"status"`
		CreateTime int64  `json:"create_time"`
		UpdateTime int64  `json:"update_time"`
		ExternalId string `json:"external_id"`
	} `json:"data"`
}

// ============================
// Adaptor implementation
// ============================

type TaskAdaptor struct {
	taskcommon.BaseBilling
	ChannelType int
	apiKey      string
	baseURL     string
}

func (a *TaskAdaptor) Init(info *relaycommon.RelayInfo) {
	a.ChannelType = info.ChannelType
	a.baseURL = info.ChannelBaseUrl
	a.apiKey = info.ApiKey

	// apiKey format: "access_key|secret_key"
}

// ValidateRequestAndSetAction parses body, validates fields and sets default action.
func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) (taskErr *dto.TaskError) {
	// Use the standard validation method for TaskSubmitReq
	return relaycommon.ValidateBasicTaskRequest(c, info, constant.TaskActionGenerate)
}

// BuildRequestURL constructs the upstream URL.
//
//   v3 models: POST /text-to-video/{api-model-id}  (T2V)
//              POST /image-to-video/{api-model-id}  (I2V)
//   v1/v2:     POST /v1/videos/text2video
//              POST /v1/videos/image2video
func (a *TaskAdaptor) BuildRequestURL(info *relaycommon.RelayInfo) (string, error) {
	isI2V := info.Action == constant.TaskActionGenerate

	if isKlingCNYModel(info.OriginModelName) {
		apiModelId := klingV3APIModelId(info.UpstreamModelName, info.OriginModelName)
		prefix := lo.Ternary(isI2V, "/image-to-video/", "/text-to-video/")
		path := prefix + apiModelId
		if isNewAPIRelay(info.ApiKey) {
			return fmt.Sprintf("%s/kling%s", a.baseURL, path), nil
		}
		return fmt.Sprintf("%s%s", a.baseURL, path), nil
	}

	path := lo.Ternary(isI2V, "/v1/videos/image2video", "/v1/videos/text2video")
	if isNewAPIRelay(info.ApiKey) {
		return fmt.Sprintf("%s/kling%s", a.baseURL, path), nil
	}
	return fmt.Sprintf("%s%s", a.baseURL, path), nil
}

// BuildRequestHeader sets required headers.
// If JWT generation fails (e.g. key is already a bearer token, not accessKey|secretKey),
// fall back to using the raw key directly — same behaviour as FetchTask.
func (a *TaskAdaptor) BuildRequestHeader(c *gin.Context, req *http.Request, info *relaycommon.RelayInfo) error {
	token, err := a.createJWTToken()
	if err != nil {
		token = a.apiKey
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("User-Agent", "kling-sdk/1.0")
	return nil
}

// BuildRequestBody converts request into Kling specific format.
//
// Routing:
//   - kling-v3-* → v3RequestPayload  (no model field, resolution-based quality)
//   - kling-v1 / kling-v2 → requestPayload (model_name, mode-based quality)
//
// The action (text2video vs image2video) is written to the gin context so that
// DoRequest → DoTaskApiRequest → BuildRequestURL can select the correct path.
func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	v, exists := c.Get("task_request")
	if !exists {
		return nil, fmt.Errorf("request not found in context")
	}
	req := v.(relaycommon.TaskSubmitReq)

	upstreamModel := info.UpstreamModelName
	if upstreamModel == "" {
		upstreamModel = info.OriginModelName
	}

	if isKlingCNYModel(upstreamModel) {
		return a.buildV3RequestBody(c, &req, info)
	}
	return a.buildV1V2RequestBody(c, &req, info)
}

// buildV3RequestBody builds the v3 request body.
// T2V → POST /text-to-video/{api-model-id}   (prompt at top level)
// I2V → POST /image-to-video/{api-model-id}  (contents array with first_frame)
func (a *TaskAdaptor) buildV3RequestBody(c *gin.Context, req *relaycommon.TaskSubmitReq, info *relaycommon.RelayInfo) (io.Reader, error) {
	body, err := a.convertToV3RequestPayload(req, info)
	if err != nil {
		return nil, err
	}

	// Detect I2V: either explicit contents array (native format via metadata) or
	// legacy image field that convertToV3RequestPayload already moved to contents.
	isI2V := len(body.Contents) > 0 && v3ContentsHaveFirstFrame(body.Contents)
	if isI2V {
		// Ensure aspect_ratio is absent for I2V (server derives from source image)
		if body.Settings != nil {
			body.Settings.AspectRatio = ""
		}
	} else {
		c.Set("action", constant.TaskActionTextGenerate)
	}

	data, err := common.Marshal(body)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(data), nil
}

// v3ContentsHaveFirstFrame reports whether a contents slice contains a first_frame item.
func v3ContentsHaveFirstFrame(contents []v3Content) bool {
	for _, c := range contents {
		if c.Type == "first_frame" {
			return true
		}
	}
	return false
}

// buildV1V2RequestBody builds the legacy v1/v2 request body.
func (a *TaskAdaptor) buildV1V2RequestBody(c *gin.Context, req *relaycommon.TaskSubmitReq, info *relaycommon.RelayInfo) (io.Reader, error) {
	body, err := a.convertToRequestPayload(req, info)
	if err != nil {
		return nil, err
	}
	if body.Image == "" && body.ImageTail == "" {
		c.Set("action", constant.TaskActionTextGenerate)
	}
	data, err := common.Marshal(body)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(data), nil
}

// DoRequest delegates to common helper.
func (a *TaskAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	if action := c.GetString("action"); action != "" {
		info.Action = action
	}
	return channel.DoTaskApiRequest(a, c, info, requestBody)
}

// DoResponse handles the create-task upstream response.
// Extracts the upstream task ID: v3 → data.id; v1/v2 → data.task_id / top-level task_id.
func (a *TaskAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (taskID string, taskData []byte, taskErr *dto.TaskError) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		taskErr = service.TaskErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
		return
	}

	// Try v3 create response first (data.id present).
	var v3Resp v3CreateResponse
	if err = common.Unmarshal(responseBody, &v3Resp); err == nil && v3Resp.Data.Id != "" {
		if v3Resp.Code != 0 {
			taskErr = service.TaskErrorWrapperLocal(fmt.Errorf("%s", v3Resp.Message), "task_failed", http.StatusBadRequest)
			return
		}
		ov := dto.NewOpenAIVideo()
		ov.ID = info.PublicTaskID
		ov.TaskID = info.PublicTaskID
		ov.CreatedAt = time.Now().Unix()
		ov.Model = info.OriginModelName
		c.JSON(http.StatusOK, ov)
		return v3Resp.Data.Id, responseBody, nil
	}

	// Fall back to v1/v2.
	var kResp responsePayload
	if err = common.Unmarshal(responseBody, &kResp); err != nil {
		taskErr = service.TaskErrorWrapper(err, "unmarshal_response_failed", http.StatusInternalServerError)
		return
	}
	if kResp.Code != 0 {
		taskErr = service.TaskErrorWrapperLocal(fmt.Errorf("%s", kResp.Message), "task_failed", http.StatusBadRequest)
		return
	}
	upstreamTaskID := kResp.Data.TaskId
	if upstreamTaskID == "" {
		upstreamTaskID = kResp.TaskId
	}
	ov := dto.NewOpenAIVideo()
	ov.ID = info.PublicTaskID
	ov.TaskID = info.PublicTaskID
	ov.CreatedAt = time.Now().Unix()
	ov.Model = info.OriginModelName
	c.JSON(http.StatusOK, ov)
	return upstreamTaskID, responseBody, nil
}

// FetchTask fetches task status from the upstream API.
//
// body must contain:
//   "task_id"    – upstream task ID
//   "action"     – constant.TaskActionGenerate (i2v) or TaskActionTextGenerate (t2v)
//   "model_name" – origin model name (used to detect v3 vs v1/v2)
//
// v3:    GET /tasks?task_ids={taskID}
// v1/v2: GET /v1/videos/text2video/{taskID}
//        GET /v1/videos/image2video/{taskID}
func (a *TaskAdaptor) FetchTask(baseUrl, key string, body map[string]any, proxy string) (*http.Response, error) {
	taskID, ok := body["task_id"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid task_id")
	}
	action, _ := body["action"].(string)
	modelName, _ := body["model_name"].(string)
	isI2V := action == constant.TaskActionGenerate

	var url string
	if isKlingCNYModel(modelName) {
		// v3: unified task query endpoint, model name not in the path
		path := fmt.Sprintf("/tasks?task_ids=%s", taskID)
		if isNewAPIRelay(key) {
			url = fmt.Sprintf("%s/kling%s", baseUrl, path)
		} else {
			url = fmt.Sprintf("%s%s", baseUrl, path)
		}
	} else {
		path := lo.Ternary(isI2V, "/v1/videos/image2video", "/v1/videos/text2video")
		if isNewAPIRelay(key) {
			url = fmt.Sprintf("%s/kling%s/%s", baseUrl, path, taskID)
		} else {
			url = fmt.Sprintf("%s%s/%s", baseUrl, path, taskID)
		}
	}

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	token, err := a.createJWTTokenWithKey(key)
	if err != nil {
		token = key
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("User-Agent", "kling-sdk/1.0")

	client, err := service.GetHttpClientWithProxy(proxy)
	if err != nil {
		return nil, fmt.Errorf("new proxy http client failed: %w", err)
	}
	return client.Do(req)
}

func (a *TaskAdaptor) GetModelList() []string {
	return []string{"kling-v1", "kling-v1-6", "kling-v2-master", "kling-v3-turbo"}
}

// EstimateBilling pre-estimates billing for kling-v3 CNY per-second models.
// Returns OtherRatios: {"seconds": duration, "resolution-720P": 1.0} or {"seconds": duration, "resolution-1080P": 1.25}.
// v3 API uses req.Resolution ("720p"/"1080p"); v1/v2 uses req.Mode ("std"/"pro").
func (a *TaskAdaptor) EstimateBilling(c *gin.Context, info *relaycommon.RelayInfo) map[string]float64 {
	if !isKlingCNYModel(info.OriginModelName) {
		return nil
	}
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return nil
	}

	duration := float64(taskcommon.DefaultInt(req.Duration, 5))
	if sec, err2 := strconv.Atoi(req.Seconds); err2 == nil && sec > 0 {
		duration = float64(sec)
	}

	// v3 models use resolution field ("720p"/"1080p"); detect 1080p explicitly.
	is1080P := strings.ToLower(strings.TrimSpace(req.Resolution)) == "1080p"

	ratios := map[string]float64{"seconds": duration}
	if is1080P {
		ratios["resolution-1080P"] = klingResolution1080PRatio
	} else {
		ratios["resolution-720P"] = 1.0
	}
	return ratios
}

// AdjustBillingOnComplete settles kling-v3 tasks based on the actual billed seconds
// returned in FinalUnitDeduction. Returns 0 for non-kling-v3 models or on parse error.
func (a *TaskAdaptor) AdjustBillingOnComplete(task *model.Task, _ *relaycommon.TaskInfo) int {
	modelName := task.Properties.OriginModelName
	if bc := task.PrivateData.BillingContext; bc != nil && bc.OriginModelName != "" {
		modelName = bc.OriginModelName
	}
	if !isKlingCNYModel(modelName) {
		return 0
	}

	var kResp responsePayload
	if err := common.Unmarshal(task.Data, &kResp); err != nil || kResp.Data.FinalUnitDeduction == "" {
		return 0
	}
	actualSeconds, err := strconv.ParseFloat(kResp.Data.FinalUnitDeduction, 64)
	if err != nil || actualSeconds <= 0 {
		return 0
	}

	bc := task.PrivateData.BillingContext
	if bc == nil || bc.ModelPrice <= 0 || bc.GroupRatio <= 0 {
		return 0
	}

	// Reconstruct resolution multiplier from what was stored at pre-consume time.
	resolutionRatio := 1.0
	for k, v := range bc.OtherRatios {
		if strings.HasPrefix(k, "resolution-") && v > 0 {
			resolutionRatio = v
			break
		}
	}

	actualQuota := int(ratio_setting.ModelPriceToUSD(modelName, bc.ModelPrice) *
		common.QuotaPerUnit * bc.GroupRatio * actualSeconds * resolutionRatio)
	return actualQuota
}

func (a *TaskAdaptor) GetChannelName() string {
	return "kling"
}

// ============================
// helpers
// ============================

func (a *TaskAdaptor) convertToRequestPayload(req *relaycommon.TaskSubmitReq, info *relaycommon.RelayInfo) (*requestPayload, error) {
	r := requestPayload{
		Prompt:         req.Prompt,
		Image:          req.Image,
		Mode:           taskcommon.DefaultString(req.Mode, "std"),
		Duration:       fmt.Sprintf("%d", taskcommon.DefaultInt(req.Duration, 5)),
		AspectRatio:    a.getAspectRatio(req.Size),
		ModelName:      info.UpstreamModelName,
		Model:          info.UpstreamModelName,
		CfgScale:       0.5,
		StaticMask:     "",
		DynamicMasks:   []DynamicMask{},
		CameraControl:  nil,
		CallbackUrl:    "",
		ExternalTaskId: "",
	}
	if r.ModelName == "" {
		r.ModelName = "kling-v1"
		r.Model = "kling-v1"
	}
	if err := taskcommon.UnmarshalMetadata(req.Metadata, &r); err != nil {
		return nil, errors.Wrap(err, "unmarshal metadata failed")
	}
	return &r, nil
}

// convertToV3RequestPayload builds the Kling v3 request body.
//
// T2V (no image):
//
//	{ "prompt": "...", "settings": {duration, resolution, aspect_ratio}, "options": {...} }
//
// I2V (req.Image set or contents provided via metadata):
//
//	{ "contents": [{type:"prompt",text:"..."},{type:"first_frame",url:"..."}],
//	  "settings": {duration, resolution},   // no aspect_ratio
//	  "options": {...} }
//
// Metadata (req.Metadata) can override any field after defaults are applied.
func (a *TaskAdaptor) convertToV3RequestPayload(req *relaycommon.TaskSubmitReq, _ *relaycommon.RelayInfo) (*v3RequestPayload, error) {
	// duration: prefer req.Seconds (string) over req.Duration (int), clamp 3–15
	duration := taskcommon.DefaultInt(req.Duration, 5)
	if sec, err := strconv.Atoi(req.Seconds); err == nil && sec > 0 {
		duration = sec
	}
	if duration < 3 {
		duration = 3
	}
	if duration > 15 {
		duration = 15
	}

	// resolution: "720p" (¥0.8/s default) or "1080p" (¥1.0/s)
	resolution := strings.ToLower(strings.TrimSpace(req.Resolution))
	if resolution == "" {
		resolution = "720p"
	}

	r := v3RequestPayload{
		Options: &v3Options{
			WatermarkInfo: &v3WatermarkInfo{Enabled: false},
		},
	}

	if req.Image != "" {
		// I2V: build contents array; aspect_ratio is determined by source image
		r.Contents = []v3Content{}
		if req.Prompt != "" {
			r.Contents = append(r.Contents, v3Content{Type: "prompt", Text: req.Prompt})
		}
		r.Contents = append(r.Contents, v3Content{Type: "first_frame", Url: req.Image})
		r.Settings = &v3Settings{
			Duration:   duration,
			Resolution: resolution,
			// no AspectRatio for I2V
		}
	} else {
		// T2V: top-level prompt + aspect_ratio in settings
		r.Prompt = req.Prompt
		r.Settings = &v3Settings{
			Duration:    duration,
			Resolution:  resolution,
			AspectRatio: a.aspectRatioFromSize(req.Size),
		}
	}

	// Metadata can override any field, e.g.:
	//   Native I2V body: {"contents":[...], "settings":{...}, "options":{...}}
	//   Custom overrides: {"options":{"callback_url":"..."}}
	if err := taskcommon.UnmarshalMetadata(req.Metadata, &r); err != nil {
		return nil, errors.Wrap(err, "unmarshal metadata failed")
	}
	return &r, nil
}

// aspectRatioFromSize converts an OpenAI-style "WxH" size string to a Kling aspect_ratio string.
// Used by both v1/v2 and v3. Falls back to "16:9" for unknown values.
func (a *TaskAdaptor) aspectRatioFromSize(size string) string {
	switch size {
	case "1024x1024", "512x512":
		return "1:1"
	case "1280x720", "1920x1080":
		return "16:9"
	case "720x1280", "1080x1920":
		return "9:16"
	default:
		return "16:9"
	}
}

// getAspectRatio is kept for v1/v2 convertToRequestPayload compatibility.
// It differs from aspectRatioFromSize only in the default (v1/v2 default is "1:1").
func (a *TaskAdaptor) getAspectRatio(size string) string {
	switch size {
	case "1024x1024", "512x512":
		return "1:1"
	case "1280x720", "1920x1080":
		return "16:9"
	case "720x1280", "1080x1920":
		return "9:16"
	default:
		return "1:1"
	}
}

// ============================
// JWT helpers
// ============================

func (a *TaskAdaptor) createJWTToken() (string, error) {
	return a.createJWTTokenWithKey(a.apiKey)
}

func (a *TaskAdaptor) createJWTTokenWithKey(apiKey string) (string, error) {
	if isNewAPIRelay(apiKey) {
		return apiKey, nil // new api relay
	}
	keyParts := strings.Split(apiKey, "|")
	if len(keyParts) != 2 {
		return "", errors.New("invalid api_key, required format is accessKey|secretKey")
	}
	accessKey := strings.TrimSpace(keyParts[0])
	if len(keyParts) == 1 {
		return accessKey, nil
	}
	secretKey := strings.TrimSpace(keyParts[1])
	now := time.Now().Unix()
	claims := jwt.MapClaims{
		"iss": accessKey,
		"exp": now + 1800, // 30 minutes
		"nbf": now - 5,
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	token.Header["typ"] = "JWT"
	return token.SignedString([]byte(secretKey))
}

// ParseTaskResult parses the polling response for both v1/v2 and v3.
//
//   v3:   GET /tasks?task_ids=xxx → data is JSON array  → try v3QueryResponse first
//   v1/v2: data is JSON object    → fall back to responsePayload
//
// v3 status:   "submitted" | "processing" | "succeeded" | "failed"
// v1/v2 status: "submitted" | "processing" | "succeed"  | "failed"
func (a *TaskAdaptor) ParseTaskResult(respBody []byte) (*relaycommon.TaskInfo, error) {
	taskInfo := &relaycommon.TaskInfo{}

	// Try v3 format: data is an array.
	v3Resp := v3QueryResponse{}
	if err := common.Unmarshal(respBody, &v3Resp); err == nil && len(v3Resp.Data) > 0 {
		taskInfo.Code = v3Resp.Code
		item := &v3Resp.Data[0]
		taskInfo.TaskID = item.Id
		taskInfo.Reason = item.Message
		switch item.Status {
		case "submitted":
			taskInfo.Status = model.TaskStatusSubmitted
		case "processing":
			taskInfo.Status = model.TaskStatusInProgress
		case "succeeded":
			taskInfo.Status = model.TaskStatusSuccess
			if vid := item.firstVideoOutput(); vid != nil {
				taskInfo.Url = vid.Url
			}
		case "failed":
			taskInfo.Status = model.TaskStatusFailure
		default:
			return nil, fmt.Errorf("unknown v3 task status: %s", item.Status)
		}
		return taskInfo, nil
	}

	// Fall back to v1/v2 format: data is an object.
	resPayload := responsePayload{}
	if err := common.Unmarshal(respBody, &resPayload); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal response body")
	}
	taskInfo.Code = resPayload.Code
	taskInfo.TaskID = resPayload.Data.TaskId
	taskInfo.Reason = resPayload.Data.TaskStatusMsg
	switch resPayload.Data.TaskStatus {
	case "submitted":
		taskInfo.Status = model.TaskStatusSubmitted
	case "processing":
		taskInfo.Status = model.TaskStatusInProgress
	case "succeed":
		taskInfo.Status = model.TaskStatusSuccess
		if videos := resPayload.Data.TaskResult.Videos; len(videos) > 0 {
			taskInfo.Url = videos[0].Url
		}
		if tokens, err := strconv.ParseFloat(resPayload.Data.FinalUnitDeduction, 64); err == nil {
			if rounded := int(math.Ceil(tokens)); rounded > 0 {
				taskInfo.CompletionTokens = rounded
				taskInfo.TotalTokens = rounded
			}
		}
	case "failed":
		taskInfo.Status = model.TaskStatusFailure
	default:
		return nil, fmt.Errorf("unknown task status: %s", resPayload.Data.TaskStatus)
	}
	return taskInfo, nil
}

// klingV3APIModelId maps an internal model name to the Kling API model identifier
// embedded in the URL path.  Falls back to upstreamName if it differs from originName.
//
//	kling-v3-turbo → kling-3.0-turbo
func klingV3APIModelId(upstreamName, originName string) string {
	// If the channel has a custom mapping, use it as-is.
	if upstreamName != "" && upstreamName != originName {
		return upstreamName
	}
	// Built-in name table.
	switch strings.ToLower(strings.TrimSpace(originName)) {
	case "kling-v3-turbo":
		return "kling-3.0-turbo"
	default:
		// Best effort: use origin name (works if the user already named it correctly).
		return originName
	}
}

func isNewAPIRelay(apiKey string) bool {
	return strings.HasPrefix(apiKey, "sk-")
}

func (a *TaskAdaptor) ConvertToOpenAIVideo(originTask *model.Task) ([]byte, error) {
	openAIVideo := dto.NewOpenAIVideo()
	openAIVideo.ID = originTask.TaskID
	openAIVideo.Status = originTask.Status.ToVideoStatus()
	openAIVideo.SetProgressStr(originTask.Progress)

	// Try v3 format first: data is an array.
	v3Resp := v3QueryResponse{}
	if err := common.Unmarshal(originTask.Data, &v3Resp); err == nil && len(v3Resp.Data) > 0 {
		item := &v3Resp.Data[0]
		openAIVideo.CreatedAt = item.CreateTime / 1000 // ms → s
		openAIVideo.CompletedAt = item.UpdateTime / 1000
		if vid := item.firstVideoOutput(); vid != nil {
			if vid.Url != "" {
				openAIVideo.SetMetadata("url", vid.Url)
			}
			if vid.Duration != "" {
				openAIVideo.Seconds = vid.Duration
			}
		}
		if v3Resp.Code != 0 && v3Resp.Message != "" {
			openAIVideo.Error = &dto.OpenAIVideoError{
				Message: v3Resp.Message,
				Code:    fmt.Sprintf("%d", v3Resp.Code),
			}
		} else if item.Status == "failed" && item.Message != "" {
			openAIVideo.Error = &dto.OpenAIVideoError{Message: item.Message}
		}
		return common.Marshal(openAIVideo)
	}

	// Fall back to v1/v2 format.
	var klingResp responsePayload
	if err := common.Unmarshal(originTask.Data, &klingResp); err != nil {
		return nil, errors.Wrap(err, "unmarshal kling task data failed")
	}
	openAIVideo.CreatedAt = klingResp.Data.CreatedAt
	openAIVideo.CompletedAt = klingResp.Data.UpdatedAt
	if videos := klingResp.Data.TaskResult.Videos; len(videos) > 0 {
		if videos[0].Url != "" {
			openAIVideo.SetMetadata("url", videos[0].Url)
		}
		if videos[0].Duration != "" {
			openAIVideo.Seconds = videos[0].Duration
		}
	}
	if klingResp.Code != 0 && klingResp.Message != "" {
		openAIVideo.Error = &dto.OpenAIVideoError{
			Message: klingResp.Message,
			Code:    fmt.Sprintf("%d", klingResp.Code),
		}
	} else if klingResp.Data.TaskStatus == "failed" {
		openAIVideo.Error = &dto.OpenAIVideoError{Message: klingResp.Data.TaskStatusMsg}
	}
	return common.Marshal(openAIVideo)
}
