package vidu

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel"
	taskcommon "github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"github.com/pkg/errors"
)

// viduCreditUnitPriceCNY is the CNY price charged per Vidu API credit (no markup,
// same as Vidu's official rate: ¥500 → 16000 credits → ¥0.03125/credit).
// Final billing uses upstream credits × this unit price (AdjustBillingOnSubmit).
// See: https://platform.vidu.cn/docs/pricing
const viduCreditUnitPriceCNY = 0.03125

// viduCreditRate holds official per-second credit rates (off-peak / peak).
type viduCreditRate struct {
	offPeak int
	peak    int
}

// viduOfficialCreditRate returns rates from https://platform.vidu.cn/docs/pricing
// reference=true for 参考生视频, false for 文生/图生/首尾帧.
func viduOfficialCreditRate(model, resolution string, reference bool) viduCreditRate {
	model = trimViduModelName(model)
	resolution = strings.ToLower(strings.TrimSpace(resolution))
	if resolution == "" {
		resolution = "720p"
	}

	if reference {
		switch model {
		case "viduq3-mix":
			if resolution == "1080p" {
				return viduCreditRate{29, 29}
			}
			return viduCreditRate{24, 24}
		case "viduq3-turbo":
			switch resolution {
			case "540p":
				return viduCreditRate{2, 4}
			case "1080p":
				return viduCreditRate{7, 13}
			default:
				return viduCreditRate{5, 10}
			}
		case "viduq3":
			switch resolution {
			case "540p":
				return viduCreditRate{4, 7}
			case "1080p":
				return viduCreditRate{7, 15}
			default:
				return viduCreditRate{6, 12}
			}
		}
	} else {
		switch model {
		case "viduq3-pro":
			switch resolution {
			case "540p":
				return viduCreditRate{5, 9}
			case "1080p":
				return viduCreditRate{12, 24}
			default:
				return viduCreditRate{10, 20}
			}
		case "viduq3-turbo":
			switch resolution {
			case "540p":
				return viduCreditRate{4, 7}
			case "1080p":
				return viduCreditRate{7, 13}
			default:
				return viduCreditRate{6, 12}
			}
		case "viduq3-pro-fast":
			switch resolution {
			case "1080p":
				return viduCreditRate{8, 15}
			default:
				return viduCreditRate{6, 12}
			}
		}
	}
	return viduCreditRate{10, 20}
}

// viduEffectiveBaselineCNYPerSec returns the official 720p baseline ¥/s for the given capability.
func viduEffectiveBaselineCNYPerSec(model string, reference bool) float64 {
	base720 := viduOfficialCreditRate(model, "720p", reference)
	if base720.offPeak > 0 {
		return float64(base720.offPeak) * viduCreditUnitPriceCNY
	}
	if base720.peak > 0 {
		return float64(base720.peak) * viduCreditUnitPriceCNY
	}
	return 0
}

func viduCreditsFromTaskData(data []byte) int {
	if len(data) == 0 {
		return 0
	}
	var poll taskResultResponse
	if err := common.Unmarshal(data, &poll); err == nil && poll.Credits > 0 {
		return poll.Credits
	}
	var submit responsePayload
	if err := common.Unmarshal(data, &submit); err == nil && submit.Credits > 0 {
		return submit.Credits
	}
	return 0
}

func viduQuotaFromCredits(modelName string, credits int, groupRatio float64) int {
	if credits <= 0 || groupRatio <= 0 {
		return 0
	}
	actualCNY := float64(credits) * viduCreditUnitPriceCNY
	return int(ratio_setting.ModelPriceToUSD(modelName, actualCNY) * common.QuotaPerUnit * groupRatio)
}

// ============================
// Request / Response structures
// ============================

type subjectPayload struct {
	Name     string   `json:"name"`
	Images   []string `json:"images,omitempty"`
	Videos   []string `json:"videos,omitempty"`
	VoiceId  string   `json:"voice_id,omitempty"`
	ServerId string   `json:"server_id,omitempty"`
}

type requestPayload struct {
	Model             string           `json:"model"`
	AutoSubjects      *bool            `json:"auto_subjects,omitempty"`
	Subjects          []subjectPayload `json:"subjects,omitempty"`
	Images            []string         `json:"images,omitempty"`
	Videos            []string         `json:"videos,omitempty"`
	Prompt            string           `json:"prompt,omitempty"`
	Style             string           `json:"style,omitempty"`
	Duration          int              `json:"duration,omitempty"`
	Seed              int              `json:"seed,omitempty"`
	AspectRatio       string           `json:"aspect_ratio,omitempty"`
	Resolution        string           `json:"resolution,omitempty"`
	MovementAmplitude string           `json:"movement_amplitude,omitempty"`
	Bgm               *bool            `json:"bgm,omitempty"`
	Audio             *bool            `json:"audio,omitempty"`
	AudioType         string           `json:"audio_type,omitempty"`
	VoiceId           string           `json:"voice_id,omitempty"`
	IsRec             *bool            `json:"is_rec,omitempty"`
	OffPeak           *bool            `json:"off_peak,omitempty"`
	Watermark         *bool            `json:"watermark,omitempty"`
	WmPosition        int              `json:"wm_position,omitempty"`
	WmUrl             string           `json:"wm_url,omitempty"`
	MetaData          string           `json:"meta_data,omitempty"`
	Payload           string           `json:"payload,omitempty"`
	CallbackUrl       string           `json:"callback_url,omitempty"`
}

type responsePayload struct {
	TaskId            string   `json:"task_id"`
	State             string   `json:"state"`
	Model             string   `json:"model"`
	Images            []string `json:"images"`
	Videos            []string `json:"videos"`
	Prompt            string   `json:"prompt"`
	Style             string   `json:"style"`
	Duration          int      `json:"duration"`
	Seed              int      `json:"seed"`
	AspectRatio       string   `json:"aspect_ratio"`
	Resolution        string   `json:"resolution"`
	Bgm               bool     `json:"bgm"`
	Audio             bool     `json:"audio"`
	AudioType         string   `json:"audio_type"`
	MovementAmplitude string   `json:"movement_amplitude"`
	OffPeak           bool     `json:"off_peak"`
	Credits           int      `json:"credits"`
	Watermark         bool     `json:"watermark"`
	Payload           string   `json:"payload"`
	CreatedAt         string   `json:"created_at"`
}

type taskResultResponse struct {
	State     string     `json:"state"`
	ErrCode   string     `json:"err_code"`
	Credits   int        `json:"credits"`
	Payload   string     `json:"payload"`
	Creations []creation `json:"creations"`
}

type creation struct {
	ID       string `json:"id"`
	URL      string `json:"url"`
	CoverURL string `json:"cover_url"`
}

// ============================
// Adaptor implementation
// ============================

type TaskAdaptor struct {
	taskcommon.BaseBilling
	ChannelType int
	baseURL     string
}

func (a *TaskAdaptor) Init(info *relaycommon.RelayInfo) {
	a.ChannelType = info.ChannelType
	a.baseURL = info.ChannelBaseUrl
}

func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) *dto.TaskError {
	if err := relaycommon.ValidateBasicTaskRequest(c, info, constant.TaskActionGenerate); err != nil {
		return err
	}
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return service.TaskErrorWrapper(err, "get_task_request_failed", http.StatusBadRequest)
	}
	action := resolveViduAction(&req)
	info.Action = action

	if taskErr := validateModelActionCompatibility(&req, action); taskErr != nil {
		return taskErr
	}

	params, err := resolveViduBillingParams(&req)
	if err != nil {
		return service.TaskErrorWrapper(err, "invalid_request", http.StatusBadRequest)
	}
	if taskErr := validateViduBillingParams(params, action, req.Model); taskErr != nil {
		return taskErr
	}

	switch action {
	case constant.TaskActionGenerate:
		if len(req.Images) != 1 {
			return service.TaskErrorWrapperLocal(
				fmt.Errorf("img2video requires exactly 1 image, got %d", len(req.Images)),
				"invalid_images", http.StatusBadRequest,
			)
		}
	case constant.TaskActionFirstTailGenerate:
		if len(req.Images) != 2 {
			return service.TaskErrorWrapperLocal(
				fmt.Errorf("start-end2video requires exactly 2 images, got %d", len(req.Images)),
				"invalid_images", http.StatusBadRequest,
			)
		}
	case constant.TaskActionReferenceGenerate:
		if taskErr := validateReferenceRequest(&req); taskErr != nil {
			return taskErr
		}
	}
	return nil
}

func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	v, exists := c.Get("task_request")
	if !exists {
		return nil, fmt.Errorf("request not found in context")
	}
	req := v.(relaycommon.TaskSubmitReq)

	body, err := a.convertToRequestPayload(&req, info)
	if err != nil {
		return nil, err
	}

	data, err := common.Marshal(body)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(data), nil
}

func (a *TaskAdaptor) BuildRequestURL(info *relaycommon.RelayInfo) (string, error) {
	var path string
	switch info.Action {
	case constant.TaskActionGenerate:
		path = "/img2video"
	case constant.TaskActionFirstTailGenerate:
		path = "/start-end2video"
	case constant.TaskActionReferenceGenerate:
		path = "/reference2video"
	default:
		path = "/text2video"
	}
	return fmt.Sprintf("%s/ent/v2%s", a.baseURL, path), nil
}

func (a *TaskAdaptor) BuildRequestHeader(c *gin.Context, req *http.Request, info *relaycommon.RelayInfo) error {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Token "+info.ApiKey)
	return nil
}

func (a *TaskAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	return channel.DoTaskApiRequest(a, c, info, requestBody)
}

func (a *TaskAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (taskID string, taskData []byte, taskErr *dto.TaskError) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		taskErr = service.TaskErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
		return
	}

	var vResp responsePayload
	err = common.Unmarshal(responseBody, &vResp)
	if err != nil {
		taskErr = service.TaskErrorWrapper(errors.Wrap(err, fmt.Sprintf("%s", responseBody)), "unmarshal_response_failed", http.StatusInternalServerError)
		return
	}

	if vResp.State == "failed" {
		taskErr = service.TaskErrorWrapperLocal(fmt.Errorf("task failed"), "task_failed", http.StatusBadRequest)
		return
	}

	ov := dto.NewOpenAIVideo()
	ov.ID = info.PublicTaskID
	ov.TaskID = info.PublicTaskID
	ov.CreatedAt = time.Now().Unix()
	ov.Model = info.OriginModelName
	c.JSON(http.StatusOK, ov)
	return vResp.TaskId, responseBody, nil
}

func (a *TaskAdaptor) FetchTask(baseUrl, key string, body map[string]any, proxy string) (*http.Response, error) {
	taskID, ok := body["task_id"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid task_id")
	}

	url := fmt.Sprintf("%s/ent/v2/tasks/%s/creations", baseUrl, taskID)

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Token "+key)

	client, err := service.GetHttpClientWithProxy(proxy)
	if err != nil {
		return nil, fmt.Errorf("new proxy http client failed: %w", err)
	}
	return client.Do(req)
}

func (a *TaskAdaptor) GetModelList() []string {
	return []string{
		"viduq3-pro",
		"viduq3-turbo",
		"viduq3-pro-fast",
		"viduq3-mix",
		"viduq3",
	}
}

func (a *TaskAdaptor) GetChannelName() string {
	return "vidu"
}

// isViduCNYModel reports whether model uses CNY-per-second pricing (viduq3 family).
func isViduCNYModel(name string) bool {
	return strings.HasPrefix(strings.ToLower(name), "viduq3")
}

// EstimateBilling pre-estimates billing for viduq3 CNY per-second models.
//
// Uses official credit rates per model × capability × resolution (see viduOfficialCreditRate).
// Pre-deducts at peak rate; AdjustBillingOnSubmit settles via upstream credits ("多退少补").
func (a *TaskAdaptor) EstimateBilling(c *gin.Context, info *relaycommon.RelayInfo) map[string]float64 {
	if !isViduCNYModel(info.OriginModelName) {
		return nil
	}
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return nil
	}

	params, err := resolveViduBillingParams(&req)
	if err != nil {
		return nil
	}
	duration := float64(params.Duration)
	resolution := params.Resolution
	reference := info.Action == constant.TaskActionReferenceGenerate
	model := trimViduModelName(info.OriginModelName)

	rate := viduOfficialCreditRate(model, resolution, reference)
	base720 := viduOfficialCreditRate(model, "720p", reference)

	ratios := map[string]float64{
		"seconds": duration,
	}
	if base720.offPeak > 0 {
		resRatio := float64(rate.offPeak) / float64(base720.offPeak)
		if resRatio != 1.0 {
			ratios["credit-resolution"] = resRatio
		}
	}
	if rate.offPeak > 0 {
		peakRatio := float64(rate.peak) / float64(rate.offPeak)
		if peakRatio != 1.0 {
			ratios["peak-prededuct"] = peakRatio
		}
	}
	if info.PriceData.ModelPrice > 0 {
		effectiveBase := viduEffectiveBaselineCNYPerSec(model, reference)
		if effectiveBase > 0 {
			if factor := effectiveBase / info.PriceData.ModelPrice; factor != 1.0 {
				ratios["vidu-baseline-factor"] = factor
			}
		}
	}
	return ratios
}

// SettleSubmitQuota settles submit billing using upstream credits with a single
// int conversion (viduQuotaFromCredits), avoiding recalcQuotaFromRatios drift.
func (a *TaskAdaptor) SettleSubmitQuota(info *relaycommon.RelayInfo, taskData []byte) (int, map[string]float64, bool) {
	if !isViduCNYModel(info.OriginModelName) {
		return 0, nil, false
	}
	var resp responsePayload
	if err := common.Unmarshal(taskData, &resp); err != nil {
		return 0, nil, false
	}
	if resp.TaskId == "" || resp.Credits <= 0 {
		return 0, nil, false
	}

	groupRatio := info.PriceData.GroupRatioInfo.GroupRatio
	if groupRatio <= 0 {
		groupRatio = 1
	}
	baseModelPrice := info.PriceData.ModelPrice
	if baseModelPrice <= 0 {
		return 0, nil, false
	}

	actualCNY := float64(resp.Credits) * viduCreditUnitPriceCNY
	quota := viduQuotaFromCredits(info.OriginModelName, resp.Credits, groupRatio)
	ratios := map[string]float64{"credits-actual": actualCNY / baseModelPrice}
	return quota, ratios, true
}

// AdjustBillingOnSubmit settles billing using the actual credits returned by Vidu.
//
// Strategy: "多退少补" — EstimateBilling pre-deducts at peak rate (2× off-peak).
// SettleSubmitQuota replaces that estimate with viduQuotaFromCredits (exact quota).
//
// If credits == 0 (Vidu did not return credits yet):
//
//	Keep the conservative peak pre-deduction as-is (return nil = no change).
func (a *TaskAdaptor) AdjustBillingOnSubmit(info *relaycommon.RelayInfo, taskData []byte) map[string]float64 {
	_, ratios, ok := a.SettleSubmitQuota(info, taskData)
	if !ok {
		return nil
	}
	return ratios
}

// AdjustBillingOnComplete settles billing when the task finishes and Vidu returns credits.
// Submit responses may omit credits; the poll response includes the final amount.
func (a *TaskAdaptor) AdjustBillingOnComplete(task *model.Task, _ *relaycommon.TaskInfo) int {
	modelName := task.Properties.OriginModelName
	if bc := task.PrivateData.BillingContext; bc != nil && bc.OriginModelName != "" {
		modelName = bc.OriginModelName
	}
	if !isViduCNYModel(modelName) {
		return 0
	}

	credits := viduCreditsFromTaskData(task.Data)
	if credits <= 0 {
		return 0
	}

	bc := task.PrivateData.BillingContext
	if bc == nil || bc.GroupRatio <= 0 {
		return 0
	}

	expected := viduQuotaFromCredits(modelName, credits, bc.GroupRatio)
	if expected <= 0 {
		return 0
	}

	// Submit-time settlement via credits-actual already matches upstream credits.
	if bc.OtherRatios != nil {
		if _, settled := bc.OtherRatios["credits-actual"]; settled {
			diff := expected - task.Quota
			if diff < 0 {
				diff = -diff
			}
			if diff <= 1 {
				return 0
			}
		}
	}

	return expected
}

// ============================
// helpers
// ============================

type viduBillingParams struct {
	Duration   int
	Resolution string
}

func cloneMetadata(metadata map[string]interface{}) map[string]interface{} {
	if metadata == nil {
		return nil
	}
	cp := make(map[string]interface{}, len(metadata))
	for k, v := range metadata {
		cp[k] = v
	}
	return cp
}

// resolveViduBillingParams resolves duration/resolution the same way as upstream payload
// (top-level defaults, metadata overrides duration, resolveResolution for resolution).
func resolveViduBillingParams(req *relaycommon.TaskSubmitReq) (viduBillingParams, error) {
	scratch := requestPayload{
		Duration: taskcommon.DefaultInt(req.Duration, 5),
	}
	if err := taskcommon.UnmarshalMetadata(cloneMetadata(req.Metadata), &scratch); err != nil {
		return viduBillingParams{}, err
	}
	resolution := resolveResolution(req)
	if resolution != "" {
		scratch.Resolution = resolution
	} else if strings.TrimSpace(scratch.Resolution) == "" {
		scratch.Resolution = "720p"
	}
	if scratch.Duration <= 0 {
		scratch.Duration = 5
	}
	return viduBillingParams{
		Duration:   scratch.Duration,
		Resolution: strings.ToLower(strings.TrimSpace(scratch.Resolution)),
	}, nil
}

func viduDurationBounds(action, model string) (min, max int) {
	if action == constant.TaskActionReferenceGenerate {
		return 3, 16
	}
	if isViduQ3Model(model) {
		return 1, 16
	}
	return 1, 16
}

func viduAllowedResolutions(action, model string) map[string]struct{} {
	model = trimViduModelName(model)
	if action == constant.TaskActionReferenceGenerate && model == "viduq3-mix" {
		return map[string]struct{}{"720p": {}, "1080p": {}}
	}
	if action != constant.TaskActionReferenceGenerate && model == "viduq3-pro-fast" {
		return map[string]struct{}{"720p": {}, "1080p": {}}
	}
	return map[string]struct{}{"540p": {}, "720p": {}, "1080p": {}}
}

func validateViduBillingParams(params viduBillingParams, action, model string) *dto.TaskError {
	minDur, maxDur := viduDurationBounds(action, model)
	if params.Duration < minDur || params.Duration > maxDur {
		return service.TaskErrorWrapperLocal(
			fmt.Errorf("duration must be between %d and %d seconds, got %d", minDur, maxDur, params.Duration),
			"invalid_duration",
			http.StatusBadRequest,
		)
	}
	allowed := viduAllowedResolutions(action, model)
	if _, ok := allowed[params.Resolution]; !ok {
		return service.TaskErrorWrapperLocal(
			fmt.Errorf("resolution %q is not supported for %s on %s", params.Resolution, trimViduModelName(model), viduActionLabel(action)),
			"invalid_resolution",
			http.StatusBadRequest,
		)
	}
	return nil
}

func (a *TaskAdaptor) convertToRequestPayload(req *relaycommon.TaskSubmitReq, info *relaycommon.RelayInfo) (*requestPayload, error) {
	params, err := resolveViduBillingParams(req)
	if err != nil {
		return nil, errors.Wrap(err, "resolve billing params failed")
	}

	r := requestPayload{
		Model:             trimViduModelName(req.Model),
		Prompt:            req.Prompt,
		Duration:          params.Duration,
		Resolution:        params.Resolution,
		MovementAmplitude: "auto",
	}
	// metadata 中的同名字段会覆盖上面的默认值，支持透传所有官方参数
	if err := taskcommon.UnmarshalMetadata(req.Metadata, &r); err != nil {
		return nil, errors.Wrap(err, "unmarshal metadata failed")
	}
	// 模型名与 duration/resolution 必须与 resolveViduBillingParams 一致，不允许 metadata 覆盖
	r.Model = trimViduModelName(req.Model)
	r.Duration = params.Duration
	r.Resolution = params.Resolution

	switch info.Action {
	case constant.TaskActionTextGenerate:
		// text2video: aspect_ratio + style; no images
		// https://platform.vidu.cn/docs/text-to-video
		r.Images = nil
		if r.AspectRatio == "" {
			r.AspectRatio = "16:9"
		}
	case constant.TaskActionGenerate, constant.TaskActionFirstTailGenerate:
		// img2video / start-end2video: resolution only, no aspect_ratio/style
		// https://platform.vidu.cn/docs/image-to-video
		// https://platform.vidu.cn/docs/start-end-to-video
		r.Images = req.Images
		r.AspectRatio = ""
		r.Style = ""
	case constant.TaskActionReferenceGenerate:
		// https://platform.vidu.cn/docs/reference-to-video
		r.Style = ""
		if len(r.Subjects) > 0 {
			// 主体模式：images/videos 由 subjects 携带，顶层 images 可选
		} else {
			r.Images = req.Images
			if len(r.Videos) == 0 {
				r.Videos = referenceVideosFromMetadata(req.Metadata)
			}
		}
		if r.AspectRatio == "" {
			r.AspectRatio = "16:9"
		}
	default:
		r.Images = req.Images
	}

	// q3 models default audio=true per official docs when not explicitly set.
	if isViduQ3Model(r.Model) && r.Audio == nil {
		audioTrue := true
		r.Audio = &audioTrue
	}

	return &r, nil
}

// resolveResolution picks the Vidu resolution field from client input.
// Priority: resolution > size (when value looks like 540p/720p/1080p) > metadata.resolution.
func resolveResolution(req *relaycommon.TaskSubmitReq) string {
	if r := strings.TrimSpace(req.Resolution); r != "" {
		return strings.ToLower(r)
	}
	if s := strings.TrimSpace(req.Size); s != "" {
		lower := strings.ToLower(s)
		if strings.HasSuffix(lower, "p") && !strings.Contains(lower, "x") {
			return lower
		}
	}
	if req.Metadata != nil {
		if v, ok := req.Metadata["resolution"].(string); ok && strings.TrimSpace(v) != "" {
			return strings.ToLower(strings.TrimSpace(v))
		}
	}
	return ""
}

func isViduQ3Model(model string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(model)), "viduq3")
}

// viduQ3OfficialModelNames lists exact upstream model names (case-sensitive).
// Requests must use these names verbatim; no aliasing or normalization is applied.
var viduQ3OfficialModelNames = map[string]struct{}{
	"viduq3-pro":      {},
	"viduq3-turbo":    {},
	"viduq3-pro-fast": {},
	"viduq3-mix":      {},
	"viduq3":          {},
}

// Official model × capability matrix (viduq3 only):
// - text2video / start-end2video
// - img2video (+ viduq3-pro-fast)
// - reference2video: https://platform.vidu.cn/docs/reference-to-video
var (
	viduModelsTextStartEnd = map[string]struct{}{
		"viduq3-pro":   {},
		"viduq3-turbo": {},
	}
	viduModelsImg2Video = map[string]struct{}{
		"viduq3-pro":      {},
		"viduq3-turbo":    {},
		"viduq3-pro-fast": {},
	}
	viduModelsReferenceNonSubject = map[string]struct{}{
		"viduq3-mix":   {},
		"viduq3-turbo": {},
		"viduq3":       {},
	}
	viduModelsReferenceSubject = map[string]struct{}{
		"viduq3-turbo": {},
		"viduq3":       {},
	}
	// reference2video 且 off_peak=true（viduq3-mix 不支持错峰）
	viduModelsReferenceOffPeak = map[string]struct{}{
		"viduq3-turbo": {},
		"viduq3":       {},
	}
)

func trimViduModelName(model string) string {
	return strings.TrimSpace(model)
}

func viduModelCapabilityError(model, capability, useInstead string) *dto.TaskError {
	model = trimViduModelName(model)
	if model == "" {
		return service.TaskErrorWrapperLocal(
			fmt.Errorf("model is required"),
			"unsupported_model",
			http.StatusBadRequest,
		)
	}
	msg := fmt.Sprintf("model %s does not support %s", model, capability)
	if useInstead != "" {
		msg += ", " + useInstead
	}
	return service.TaskErrorWrapperLocal(
		fmt.Errorf("%s", msg),
		"unsupported_model",
		http.StatusBadRequest,
	)
}

func validateOfficialViduQ3ModelName(model string) *dto.TaskError {
	model = trimViduModelName(model)
	if model == "" {
		return service.TaskErrorWrapperLocal(
			fmt.Errorf("model is required"),
			"unsupported_model",
			http.StatusBadRequest,
		)
	}
	if _, ok := viduQ3OfficialModelNames[model]; !ok {
		return service.TaskErrorWrapperLocal(
			fmt.Errorf("invalid model %q, supported models: viduq3-pro, viduq3-turbo, viduq3-pro-fast, viduq3-mix, viduq3", model),
			"unsupported_model",
			http.StatusBadRequest,
		)
	}
	return nil
}

func viduModelAllowed(model string, allowed map[string]struct{}) bool {
	_, ok := allowed[trimViduModelName(model)]
	return ok
}

func validateModelActionCompatibility(req *relaycommon.TaskSubmitReq, action string) *dto.TaskError {
	if taskErr := validateOfficialViduQ3ModelName(req.Model); taskErr != nil {
		return taskErr
	}
	model := trimViduModelName(req.Model)

	switch action {
	case constant.TaskActionTextGenerate, constant.TaskActionFirstTailGenerate:
		if !viduModelAllowed(model, viduModelsTextStartEnd) {
			return viduModelCapabilityError(model, viduActionLabel(action), "use viduq3-pro or viduq3-turbo instead")
		}
	case constant.TaskActionGenerate:
		if !viduModelAllowed(model, viduModelsImg2Video) {
			return viduModelCapabilityError(model, "img2video", "use viduq3-pro, viduq3-turbo or viduq3-pro-fast instead")
		}
	case constant.TaskActionReferenceGenerate:
		return validateReferenceModelCompatibility(req, model)
	}
	return nil
}

func viduActionLabel(action string) string {
	switch action {
	case constant.TaskActionGenerate:
		return "img2video"
	case constant.TaskActionFirstTailGenerate:
		return "start-end2video"
	case constant.TaskActionReferenceGenerate:
		return "reference2video"
	default:
		return "text2video"
	}
}

func validateReferenceModelCompatibility(req *relaycommon.TaskSubmitReq, model string) *dto.TaskError {
	subjects, err := parseReferenceSubjects(req.Metadata)
	if err != nil {
		return service.TaskErrorWrapperLocal(
			errors.Wrap(err, "invalid subjects"),
			"invalid_subjects", http.StatusBadRequest,
		)
	}

	if len(referenceVideosFromMetadata(req.Metadata)) > 0 {
		return service.TaskErrorWrapperLocal(
			fmt.Errorf("reference video input is not supported for viduq3 models"),
			"unsupported_param",
			http.StatusBadRequest,
		)
	}
	for i, subject := range subjects {
		if len(subject.Videos) > 0 {
			return service.TaskErrorWrapperLocal(
				fmt.Errorf("subject videos in subjects[%d] are not supported for viduq3 models", i),
				"unsupported_param",
				http.StatusBadRequest,
			)
		}
	}

	if offPeak, ok := metadataBool(req.Metadata, "off_peak"); ok && offPeak {
		if model == "viduq3-mix" {
			return viduModelCapabilityError(model, "off_peak for reference2video", "use viduq3 or viduq3-turbo instead")
		}
		if !viduModelAllowed(model, viduModelsReferenceOffPeak) {
			return viduModelCapabilityError(model, "off_peak for reference2video", "use viduq3 or viduq3-turbo instead")
		}
	}

	if len(subjects) > 0 {
		if !viduModelAllowed(model, viduModelsReferenceSubject) {
			switch model {
			case "viduq3-mix":
				return viduModelCapabilityError(model, "reference2video with subjects", "use viduq3 or viduq3-turbo instead")
			case "viduq3-pro", "viduq3-pro-fast":
				return viduModelCapabilityError(model, "reference2video with subjects", "use viduq3 instead")
			default:
				return viduModelCapabilityError(model, "reference2video with subjects", "use viduq3 or viduq3-turbo instead")
			}
		}
		return nil
	}

	if !viduModelAllowed(model, viduModelsReferenceNonSubject) {
		switch model {
		case "viduq3-pro":
			return viduModelCapabilityError(model, "reference2video", "use viduq3 instead")
		case "viduq3-pro-fast":
			return viduModelCapabilityError(model, "reference2video", "use viduq3, viduq3-mix or viduq3-turbo instead")
		default:
			return viduModelCapabilityError(model, "reference2video", "use viduq3, viduq3-mix or viduq3-turbo instead")
		}
	}
	return nil
}

func metadataBool(metadata map[string]interface{}, key string) (bool, bool) {
	if metadata == nil {
		return false, false
	}
	raw, ok := metadata[key]
	if !ok {
		return false, false
	}
	switch v := raw.(type) {
	case bool:
		return v, true
	case string:
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "true", "1":
			return true, true
		case "false", "0":
			return false, true
		}
	}
	return false, false
}

func normalizeViduAction(action string) string {
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "reference2video", "reference", constant.TaskActionReferenceGenerate:
		return constant.TaskActionReferenceGenerate
	case "img2video", "image2video", constant.TaskActionGenerate:
		return constant.TaskActionGenerate
	case "start-end2video", "startend2video", "start_end2video", constant.TaskActionFirstTailGenerate:
		return constant.TaskActionFirstTailGenerate
	case "text2video", constant.TaskActionTextGenerate:
		return constant.TaskActionTextGenerate
	default:
		return strings.TrimSpace(action)
	}
}

func resolveViduAction(req *relaycommon.TaskSubmitReq) string {
	if req.Metadata != nil {
		if metaAction, ok := req.Metadata["action"]; ok {
			if s, ok := metaAction.(string); ok && strings.TrimSpace(s) != "" {
				return normalizeViduAction(s)
			}
		}
	}
	if hasReferenceSubjects(req.Metadata) || len(referenceVideosFromMetadata(req.Metadata)) > 0 {
		return constant.TaskActionReferenceGenerate
	}
	if req.HasImage() {
		switch len(req.Images) {
		case 1:
			return constant.TaskActionGenerate
		case 2:
			return constant.TaskActionFirstTailGenerate
		default:
			return constant.TaskActionReferenceGenerate
		}
	}
	return constant.TaskActionTextGenerate
}

func hasReferenceSubjects(metadata map[string]interface{}) bool {
	subjects, err := parseReferenceSubjects(metadata)
	return err == nil && len(subjects) > 0
}

func parseReferenceSubjects(metadata map[string]interface{}) ([]subjectPayload, error) {
	if metadata == nil {
		return nil, nil
	}
	raw, ok := metadata["subjects"]
	if !ok {
		return nil, nil
	}
	data, err := common.Marshal(raw)
	if err != nil {
		return nil, err
	}
	var subjects []subjectPayload
	if err := common.Unmarshal(data, &subjects); err != nil {
		return nil, err
	}
	return subjects, nil
}

func referenceVideosFromMetadata(metadata map[string]interface{}) []string {
	if metadata == nil {
		return nil
	}
	raw, ok := metadata["videos"]
	if !ok {
		return nil
	}
	switch v := raw.(type) {
	case []string:
		return v
	case []interface{}:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, strings.TrimSpace(s))
			}
		}
		return out
	default:
		return nil
	}
}

func validateReferenceRequest(req *relaycommon.TaskSubmitReq) *dto.TaskError {
	subjects, err := parseReferenceSubjects(req.Metadata)
	if err != nil {
		return service.TaskErrorWrapperLocal(
			errors.Wrap(err, "invalid subjects"),
			"invalid_subjects", http.StatusBadRequest,
		)
	}
	if len(subjects) > 0 {
		for i, subject := range subjects {
			if strings.TrimSpace(subject.Name) == "" {
				return service.TaskErrorWrapperLocal(
					fmt.Errorf("subjects[%d].name is required", i),
					"invalid_subjects", http.StatusBadRequest,
				)
			}
			if strings.TrimSpace(subject.ServerId) == "" && len(subject.Images) == 0 && len(subject.Videos) == 0 {
				return service.TaskErrorWrapperLocal(
					fmt.Errorf("subjects[%d] requires images, videos, or server_id", i),
					"invalid_subjects", http.StatusBadRequest,
				)
			}
			if len(subject.Images) > 3 {
				return service.TaskErrorWrapperLocal(
					fmt.Errorf("subjects[%d] supports at most 3 images", i),
					"invalid_subjects", http.StatusBadRequest,
				)
			}
		}
		return nil
	}

	imageCount := len(req.Images)
	videoCount := len(referenceVideosFromMetadata(req.Metadata))
	if imageCount == 0 && videoCount == 0 {
		return service.TaskErrorWrapperLocal(
			fmt.Errorf("reference2video requires 1-7 images or 1-2 videos in metadata"),
			"invalid_reference_input", http.StatusBadRequest,
		)
	}
	if imageCount > 7 {
		return service.TaskErrorWrapperLocal(
			fmt.Errorf("reference2video supports at most 7 images, got %d", imageCount),
			"invalid_images", http.StatusBadRequest,
		)
	}
	if videoCount > 2 {
		return service.TaskErrorWrapperLocal(
			fmt.Errorf("reference2video supports at most 2 videos, got %d", videoCount),
			"invalid_videos", http.StatusBadRequest,
		)
	}
	if videoCount > 0 {
		return service.TaskErrorWrapperLocal(
			fmt.Errorf("reference video input is not supported for viduq3 models"),
			"unsupported_param",
			http.StatusBadRequest,
		)
	}
	return nil
}

func (a *TaskAdaptor) ParseTaskResult(respBody []byte) (*relaycommon.TaskInfo, error) {
	taskInfo := &relaycommon.TaskInfo{}

	var taskResp taskResultResponse
	err := common.Unmarshal(respBody, &taskResp)
	if err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal response body")
	}

	state := taskResp.State
	switch state {
	case "created", "queueing":
		taskInfo.Status = model.TaskStatusSubmitted
	case "processing":
		taskInfo.Status = model.TaskStatusInProgress
	case "success":
		taskInfo.Status = model.TaskStatusSuccess
		if len(taskResp.Creations) > 0 {
			taskInfo.Url = taskResp.Creations[0].URL
		}
	case "failed":
		taskInfo.Status = model.TaskStatusFailure
		if taskResp.ErrCode != "" {
			taskInfo.Reason = taskResp.ErrCode
		}
	default:
		return nil, fmt.Errorf("unknown task state: %s", state)
	}

	return taskInfo, nil
}

func (a *TaskAdaptor) ConvertToOpenAIVideo(originTask *model.Task) ([]byte, error) {
	var viduResp taskResultResponse
	if err := common.Unmarshal(originTask.Data, &viduResp); err != nil {
		return nil, errors.Wrap(err, "unmarshal vidu task data failed")
	}

	openAIVideo := dto.NewOpenAIVideo()
	openAIVideo.ID = originTask.TaskID
	openAIVideo.Status = originTask.Status.ToVideoStatus()
	openAIVideo.SetProgressStr(originTask.Progress)
	openAIVideo.CreatedAt = originTask.CreatedAt
	openAIVideo.CompletedAt = originTask.UpdatedAt

	if len(viduResp.Creations) > 0 && viduResp.Creations[0].URL != "" {
		openAIVideo.SetMetadata("url", viduResp.Creations[0].URL)
	}

	if viduResp.State == "failed" && viduResp.ErrCode != "" {
		openAIVideo.Error = &dto.OpenAIVideoError{
			Message: viduResp.ErrCode,
			Code:    viduResp.ErrCode,
		}
	}

	return common.Marshal(openAIVideo)
}
