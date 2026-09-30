package ali

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/samber/lo"

	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
)

// ============================
// Request / Response structures
// ============================

// AliVideoRequest 阿里通义万相视频生成请求
type AliVideoRequest struct {
	Model      string              `json:"model"`
	Input      AliVideoInput       `json:"input"`
	Parameters *AliVideoParameters `json:"parameters,omitempty"`
}

// AliVideoInput 视频输入参数
type AliVideoInput struct {
	Prompt         string          `json:"prompt,omitempty"`          // 文本提示词
	ImgURL         string          `json:"img_url,omitempty"`         // 首帧图像URL或Base64（图生视频）
	FirstFrameURL  string          `json:"first_frame_url,omitempty"` // 首帧图片URL（首尾帧生视频）
	LastFrameURL   string          `json:"last_frame_url,omitempty"`  // 尾帧图片URL（首尾帧生视频）
	AudioURL       string          `json:"audio_url,omitempty"`       // 音频URL（wan2.5支持）
	NegativePrompt string          `json:"negative_prompt,omitempty"` // 反向提示词
	Template       string          `json:"template,omitempty"`        // 视频特效模板
	Media          []AliVideoMidia `json:"media,omitempty"`           // 参考素材数组（r2v 等）：first_frame / reference_image / reference_video
}

// AliVideoMidia 参考媒体项
type AliVideoMidia struct {
	Type string `json:"type,omitempty"`
	Url  string `json:"url,omitempty"`
}

// AliVideoParameters 视频参数
type AliVideoParameters struct {
	Resolution   string `json:"resolution,omitempty"`    // 分辨率: 480P/720P/1080P（图生视频、首尾帧生视频）
	Size         string `json:"size,omitempty"`          // 尺寸: 如 "832*480"（文生视频）
	Duration     int    `json:"duration,omitempty"`      // 时长: 3-10秒
	PromptExtend *bool  `json:"prompt_extend,omitempty"` // 是否开启prompt智能改写
	Watermark    *bool  `json:"watermark,omitempty"`     // 是否添加水印
	Audio        *bool  `json:"audio,omitempty"`         // 是否添加音频（wan2.5）
	Seed         *int   `json:"seed,omitempty"`          // 随机数种子
	Ratio        string `json:"ratio,omitempty"`         // 宽高比: 如 "16:9"（文生视频）
}

// AliVideoResponse 阿里通义万相响应
type AliVideoResponse struct {
	Output    AliVideoOutput `json:"output"`
	RequestID string         `json:"request_id"`
	Code      string         `json:"code,omitempty"`
	Message   string         `json:"message,omitempty"`
	Usage     *AliUsage      `json:"usage,omitempty"`
}

// AliVideoOutput 输出信息
type AliVideoOutput struct {
	TaskID        string `json:"task_id"`
	TaskStatus    string `json:"task_status"`
	SubmitTime    string `json:"submit_time,omitempty"`
	ScheduledTime string `json:"scheduled_time,omitempty"`
	EndTime       string `json:"end_time,omitempty"`
	OrigPrompt    string `json:"orig_prompt,omitempty"`
	ActualPrompt  string `json:"actual_prompt,omitempty"`
	VideoURL      string `json:"video_url,omitempty"`
	Code          string `json:"code,omitempty"`
	Message       string `json:"message,omitempty"`
}

// AliUsage 使用统计
// duration / input_video_duration / output_video_duration are floats for video-edit models.
type AliUsage struct {
	Duration            float64      `json:"duration,omitempty"`
	InputVideoDuration  float64      `json:"input_video_duration,omitempty"`
	OutputVideoDuration float64      `json:"output_video_duration,omitempty"`
	VideoCount          dto.IntValue `json:"video_count,omitempty"`
	SR                  dto.IntValue `json:"SR,omitempty"`
}

type AliMetadata struct {
	// Input 相关
	AudioURL       string          `json:"audio_url,omitempty"`       // 音频URL
	ImgURL         string          `json:"img_url,omitempty"`         // 图片URL（图生视频）
	FirstFrameURL  string          `json:"first_frame_url,omitempty"` // 首帧图片URL（首尾帧生视频）
	LastFrameURL   string          `json:"last_frame_url,omitempty"`  // 尾帧图片URL（首尾帧生视频）
	NegativePrompt string          `json:"negative_prompt,omitempty"` // 反向提示词
	Template       string          `json:"template,omitempty"`        // 视频特效模板
	Media          []AliVideoMidia `json:"media,omitempty"`           // 参考素材数组

	// Parameters 相关
	Resolution   *string `json:"resolution,omitempty"`    // 分辨率: 480P/720P/1080P
	Size         *string `json:"size,omitempty"`          // 尺寸: 如 "832*480"
	Ratio        *string `json:"ratio,omitempty"`         // 宽高比: 16:9 / 9:16 / 1:1 等
	Duration     *int    `json:"duration,omitempty"`      // 时长
	PromptExtend *bool   `json:"prompt_extend,omitempty"` // 是否开启prompt智能改写
	Watermark    *bool   `json:"watermark,omitempty"`     // 是否添加水印
	Audio        *bool   `json:"audio,omitempty"`         // 是否添加音频
	Seed         *int    `json:"seed,omitempty"`          // 随机数种子
}

type aliMetadataEnvelope struct {
	Input      *AliVideoInput      `json:"input,omitempty"`
	Parameters *AliVideoParameters `json:"parameters,omitempty"`
	AliMetadata
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
}

func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) (taskErr *dto.TaskError) {
	// ValidateMultipartDirect 负责解析并将原始 TaskSubmitReq 存入 context
	return relaycommon.ValidateMultipartDirect(c, info)
}

func (a *TaskAdaptor) BuildRequestURL(info *relaycommon.RelayInfo) (string, error) {
	return fmt.Sprintf("%s/api/v1/services/aigc/video-generation/video-synthesis", a.baseURL), nil
}

// BuildRequestHeader sets required headers for Ali API
func (a *TaskAdaptor) BuildRequestHeader(c *gin.Context, req *http.Request, info *relaycommon.RelayInfo) error {
	req.Header.Set("Authorization", "Bearer "+a.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-DashScope-Async", "enable") // 阿里异步任务必须设置
	return nil
}

func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	taskReq, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return nil, errors.Wrap(err, "get_task_request_failed")
	}

	aliReq, err := a.convertToAliRequest(info, taskReq)
	if err != nil {
		return nil, errors.Wrap(err, "convert_to_ali_request_failed")
	}
	logger.LogJson(c, "ali video request body", aliReq)

	bodyBytes, err := common.Marshal(aliReq)
	if err != nil {
		return nil, errors.Wrap(err, "marshal_ali_request_failed")
	}
	return bytes.NewReader(bodyBytes), nil
}

var (
	size480p = []string{
		"832*480",
		"480*832",
		"624*624",
	}
	size720p = []string{
		"1280*720",
		"720*1280",
		"960*960",
		"1088*832",
		"832*1088",
	}
	size1080p = []string{
		"1920*1080",
		"1080*1920",
		"1440*1440",
		"1632*1248",
		"1248*1632",
	}
)

func sizeToResolution(size string) (string, error) {
	if lo.Contains(size480p, size) {
		return "480P", nil
	} else if lo.Contains(size720p, size) {
		return "720P", nil
	} else if lo.Contains(size1080p, size) {
		return "1080P", nil
	}
	return "", fmt.Errorf("invalid size: %s", size)
}

// happyHorseResolutionRatios returns resolution multipliers relative to 720P baseline price (0.9 CNY/sec).
func happyHorseResolutionRatios(model string) map[string]float64 {
	ratio1080 := 16.0 / 9.0 // 1.0 series: 1.6 / 0.9
	if strings.Contains(model, "1.1") {
		ratio1080 = 4.0 / 3.0 // 1.1 series: 1.2 / 0.9
	}
	return map[string]float64{
		"720P":  1,
		"1080P": ratio1080,
	}
}

func isHappyHorseModel(model string) bool {
	return strings.Contains(strings.ToLower(model), "happyhorse")
}

func isWan3Model(model string) bool {
	switch strings.ToLower(strings.TrimSpace(model)) {
	case "wan3.0-video", "wan3.0-video-prime":
		return true
	default:
		return false
	}
}

func isHappyHorseModelType(model, modelType string) bool {
	model = strings.ToLower(strings.TrimSpace(model))
	return isHappyHorseModel(model) && strings.Contains(model, "-"+modelType)
}

func metadataIntValue(v interface{}) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case int64:
		return int(n)
	case string:
		if i, err := strconv.Atoi(strings.TrimSpace(n)); err == nil {
			return i
		}
	}
	return 0
}

// happyHorseBillableSeconds returns billable video seconds; video-edit includes input duration.
func happyHorseBillableSeconds(aliReq *AliVideoRequest, taskReq relaycommon.TaskSubmitReq) float64 {
	seconds := float64(aliReq.Parameters.Duration)
	if !isHappyHorseModel(aliReq.Model) {
		return seconds
	}
	if strings.Contains(aliReq.Model, "video-edit") && taskReq.Metadata != nil {
		if inputRaw, ok := taskReq.Metadata["input_video_duration"]; ok {
			seconds += float64(metadataIntValue(inputRaw))
		}
	}
	return seconds
}

func resolveAliResolution(aliReq *AliVideoRequest) (string, error) {
	if aliReq.Parameters.Size != "" {
		return sizeToResolution(aliReq.Parameters.Size)
	}
	resolution := strings.ToUpper(aliReq.Parameters.Resolution)
	if !strings.HasSuffix(resolution, "P") {
		resolution += "P"
	}
	return resolution, nil
}

func ProcessAliOtherRatios(aliReq *AliVideoRequest, originModelName string) (map[string]float64, error) {
	otherRatios := make(map[string]float64)
	resolution, err := resolveAliResolution(aliReq)
	if err != nil {
		return nil, err
	}

	if isHappyHorseModel(originModelName) || isHappyHorseModel(aliReq.Model) {
		if ratio, ok := happyHorseResolutionRatios(aliReq.Model)[resolution]; ok {
			otherRatios[fmt.Sprintf("resolution-%s", resolution)] = ratio
		}
		return otherRatios, nil
	}

	if isWan3Model(originModelName) || isWan3Model(aliReq.Model) {
		wan3Ratios := map[string]float64{
			"480P":  0.5,
			"720P":  1,
			"1080P": 2,
		}
		if ratio, ok := wan3Ratios[resolution]; ok {
			otherRatios[fmt.Sprintf("resolution-%s", resolution)] = ratio
		}
		return otherRatios, nil
	}

	aliRatios := map[string]map[string]float64{
		"wan2.6-i2v": {
			"720P":  1,
			"1080P": 1 / 0.6,
		},
		"wan2.5-t2v-preview": {
			"480P":  1,
			"720P":  2,
			"1080P": 1 / 0.3,
		},
		"wan2.2-t2v-plus": {
			"480P":  1,
			"1080P": 0.7 / 0.14,
		},
		"wan2.5-i2v-preview": {
			"480P":  1,
			"720P":  2,
			"1080P": 1 / 0.3,
		},
		"wan2.2-i2v-plus": {
			"480P":  1,
			"1080P": 0.7 / 0.14,
		},
		"wan2.2-kf2v-flash": {
			"480P":  1,
			"720P":  2,
			"1080P": 4.8,
		},
		"wan2.2-i2v-flash": {
			"480P": 1,
			"720P": 2,
		},
		"wan2.2-s2v": {
			"480P": 1,
			"720P": 0.9 / 0.5,
		},
	}
	if otherRatio, ok := aliRatios[aliReq.Model]; ok {
		if ratio, ok := otherRatio[resolution]; ok {
			otherRatios[fmt.Sprintf("resolution-%s", resolution)] = ratio
		}
	}
	return otherRatios, nil
}

var validAliRatios = []string{"16:9", "9:16", "1:1", "4:3", "3:4"}

var validWan3Ratios = []string{"adaptive", "21:9", "16:9", "4:3", "1:1", "3:4", "9:16"}

func isAliRatioFormat(s string) bool {
	s = strings.TrimSpace(s)
	return lo.Contains(validAliRatios, s)
}

func isWan3RatioFormat(s string) bool {
	return lo.Contains(validWan3Ratios, strings.ToLower(strings.TrimSpace(s)))
}

func aliUsesRatioParam(model string) bool {
	if isWan3Model(model) {
		return true
	}
	if isHappyHorseModel(model) && (strings.Contains(model, "t2v") || strings.Contains(model, "r2v")) {
		return true
	}
	if strings.Contains(model, "r2v") || strings.Contains(model, "wan2.7") {
		return true
	}
	return strings.Contains(model, "t2v") &&
		(strings.HasPrefix(model, "wan2.5") || strings.HasPrefix(model, "wan2.6") || strings.HasPrefix(model, "wan2.7"))
}

func aliUsesMediaArray(model string) bool {
	model = strings.ToLower(model)
	return isWan3Model(model) || strings.Contains(model, "r2v") || strings.Contains(model, "wan2.7") || isHappyHorseModelType(model, "i2v")
}

func collectTaskImages(req relaycommon.TaskSubmitReq) []string {
	var images []string
	if url := strings.TrimSpace(req.InputReference); url != "" {
		images = append(images, url)
	}
	if url := strings.TrimSpace(req.Image); url != "" {
		images = append(images, url)
	}
	for _, url := range req.Images {
		if trimmed := strings.TrimSpace(url); trimmed != "" {
			images = append(images, trimmed)
		}
	}
	for _, item := range req.Content {
		if item.ImageURL != nil {
			if url := strings.TrimSpace(item.ImageURL.URL); url != "" {
				images = append(images, url)
			}
		}
	}
	return lo.Uniq(images)
}

func appendWan3ContentMedia(input *AliVideoInput, req relaycommon.TaskSubmitReq) {
	content := req.Content
	if len(content) == 0 && req.Metadata != nil {
		if raw, ok := req.Metadata["content"]; ok {
			if data, err := common.Marshal(raw); err == nil {
				_ = common.Unmarshal(data, &content)
			}
		}
	}
	appendWan3ContentItems(input, content)
}

func appendWan3ContentItems(input *AliVideoInput, content []relaycommon.TaskContentItem) {
	imageCount := 0
	hasOtherMedia := false
	for _, item := range content {
		switch strings.ToLower(strings.TrimSpace(item.Type)) {
		case "image", "image_url":
			if item.ImageURL != nil {
				imageCount++
			}
		case "video", "video_url", "audio", "audio_url":
			hasOtherMedia = true
		}
	}

	for _, item := range content {
		mediaType := ""
		switch strings.ToLower(strings.TrimSpace(item.Type)) {
		case "image", "image_url":
			if imageCount == 1 && !hasOtherMedia {
				mediaType = "first_frame"
			} else {
				mediaType = "reference_image"
			}
		case "video", "video_url":
			mediaType = "reference_video"
		case "audio", "audio_url":
			mediaType = "reference_audio"
		default:
			// Some callers already provide the Wan media type in role. Ignore
			// ordinary chat roles such as "user" and "assistant".
			role := strings.ToLower(strings.TrimSpace(item.Role))
			if lo.Contains([]string{"first_frame", "last_frame", "reference_image", "reference_video", "reference_audio", "file", "link"}, role) {
				mediaType = role
			}
		}
		var url string
		switch {
		case item.ImageURL != nil:
			url = item.ImageURL.URL
		case item.VideoURL != nil:
			url = item.VideoURL.URL
		case item.AudioURL != nil:
			url = item.AudioURL.URL
		}
		if mediaType != "" && strings.TrimSpace(url) != "" {
			input.Media = append(input.Media, AliVideoMidia{Type: mediaType, Url: strings.TrimSpace(url)})
		}
	}
}

func normalizeAliResolution(resolution string) string {
	resolution = strings.ToUpper(strings.TrimSpace(resolution))
	if resolution == "" {
		return ""
	}
	if !strings.HasSuffix(resolution, "P") {
		resolution += "P"
	}
	return resolution
}

func aspectRatioFromPixelSize(size string) string {
	size = strings.TrimSpace(size)
	if size == "" {
		return ""
	}
	sep := "*"
	if strings.Contains(size, "x") {
		sep = "x"
	}
	parts := strings.Split(size, sep)
	if len(parts) != 2 {
		return ""
	}
	w, errW := strconv.Atoi(strings.TrimSpace(parts[0]))
	h, errH := strconv.Atoi(strings.TrimSpace(parts[1]))
	if errW != nil || errH != nil || w <= 0 || h <= 0 {
		return ""
	}
	switch {
	case w > h:
		return "16:9"
	case w < h:
		return "9:16"
	default:
		return "1:1"
	}
}

func applyTaskImages(aliReq *AliVideoRequest, model string, images []string) error {
	if len(images) == 0 {
		return nil
	}
	if len(aliReq.Input.Media) > 0 {
		return nil
	}

	if isHappyHorseModelType(model, "t2v") {
		return fmt.Errorf("%s does not support image inputs", model)
	}
	if isHappyHorseModelType(model, "i2v") {
		if len(images) != 1 {
			return fmt.Errorf("%s requires exactly one image input", model)
		}
		aliReq.Input.Media = []AliVideoMidia{{
			Type: "first_frame",
			Url:  images[0],
		}}
		return nil
	}
	if isHappyHorseModelType(model, "r2v") {
		for _, url := range images {
			aliReq.Input.Media = append(aliReq.Input.Media, AliVideoMidia{
				Type: "reference_image",
				Url:  url,
			})
		}
		return nil
	}
	if isWan3Model(model) {
		// The legacy images field has no media type. Treat one image as a
		// first frame, two images as first/last frames, and larger batches as
		// reference images. Callers needing mixed media should use typed media.
		if len(images) > 2 {
			for _, url := range images {
				aliReq.Input.Media = append(aliReq.Input.Media, AliVideoMidia{
					Type: "reference_image",
					Url:  url,
				})
			}
			return nil
		}
		aliReq.Input.Media = append(aliReq.Input.Media, AliVideoMidia{
			Type: "first_frame",
			Url:  images[0],
		})
		if len(images) == 2 {
			aliReq.Input.Media = append(aliReq.Input.Media, AliVideoMidia{
				Type: "last_frame",
				Url:  images[1],
			})
		}
		return nil
	}
	if aliUsesMediaArray(model) {
		for i, url := range images {
			mediaType := "reference_image"
			if i == 0 {
				mediaType = "first_frame"
			}
			aliReq.Input.Media = append(aliReq.Input.Media, AliVideoMidia{
				Type: mediaType,
				Url:  url,
			})
		}
		return nil
	}
	if strings.Contains(model, "kf2v") {
		if aliReq.Input.FirstFrameURL == "" {
			aliReq.Input.FirstFrameURL = images[0]
		}
		if len(images) > 1 && aliReq.Input.LastFrameURL == "" {
			aliReq.Input.LastFrameURL = images[1]
		}
		return nil
	}
	if aliReq.Input.ImgURL == "" {
		aliReq.Input.ImgURL = images[0]
	}
	return nil
}

func normalizeHappyHorseInput(aliReq *AliVideoRequest) error {
	if !isHappyHorseModel(aliReq.Model) {
		return nil
	}

	input := &aliReq.Input
	legacyInput := input.ImgURL != "" || input.FirstFrameURL != "" || input.LastFrameURL != ""
	switch {
	case isHappyHorseModelType(aliReq.Model, "t2v"):
		if len(input.Media) > 0 || legacyInput {
			return fmt.Errorf("%s only supports prompt input", aliReq.Model)
		}
	case isHappyHorseModelType(aliReq.Model, "i2v"):
		if len(input.Media) == 0 {
			imageURL := input.ImgURL
			if imageURL == "" {
				imageURL = input.FirstFrameURL
			}
			if imageURL != "" {
				input.Media = []AliVideoMidia{{Type: "first_frame", Url: imageURL}}
			}
		}
		if len(input.Media) == 0 {
			return fmt.Errorf("%s requires one first_frame image input", aliReq.Model)
		}
		if len(input.Media) > 1 {
			return fmt.Errorf("%s requires exactly one first_frame image input", aliReq.Model)
		}
		media := &input.Media[0]
		media.Url = strings.TrimSpace(media.Url)
		if media.Url == "" {
			return fmt.Errorf("%s requires a non-empty first_frame image URL", aliReq.Model)
		}
		mediaType := strings.ToLower(strings.TrimSpace(media.Type))
		if mediaType == "" {
			mediaType = "first_frame"
		}
		if mediaType != "first_frame" {
			return fmt.Errorf("%s requires media type first_frame", aliReq.Model)
		}
		media.Type = mediaType
		input.ImgURL = ""
		input.FirstFrameURL = ""
		input.LastFrameURL = ""
	case isHappyHorseModelType(aliReq.Model, "r2v"):
		if len(input.Media) == 0 {
			imageURL := input.ImgURL
			if imageURL == "" {
				imageURL = input.FirstFrameURL
			}
			if imageURL != "" {
				input.Media = []AliVideoMidia{{Type: "reference_image", Url: imageURL}}
			}
		}
		if len(input.Media) == 0 {
			return fmt.Errorf("%s requires at least one reference_image input", aliReq.Model)
		}
		if len(input.Media) > 10 {
			return fmt.Errorf("%s supports at most 10 reference_image inputs", aliReq.Model)
		}
		for i := range input.Media {
			media := &input.Media[i]
			media.Url = strings.TrimSpace(media.Url)
			if media.Url == "" {
				return fmt.Errorf("%s requires non-empty reference_image URLs", aliReq.Model)
			}
			mediaType := strings.ToLower(strings.TrimSpace(media.Type))
			if mediaType == "" {
				mediaType = "reference_image"
			}
			if mediaType != "reference_image" {
				return fmt.Errorf("%s requires media type reference_image", aliReq.Model)
			}
			media.Type = mediaType
		}
		input.ImgURL = ""
		input.FirstFrameURL = ""
		input.LastFrameURL = ""
	}
	return nil
}

// normalizeWan3Input converts legacy image/audio fields to the typed media
// format required by Wan 3.0 and validates the media combinations documented
// by DashScope.
func normalizeWan3Input(aliReq *AliVideoRequest) error {
	if !isWan3Model(aliReq.Model) {
		return nil
	}

	input := &aliReq.Input
	if len(input.Media) == 0 {
		firstURL := strings.TrimSpace(input.FirstFrameURL)
		if firstURL == "" {
			firstURL = strings.TrimSpace(input.ImgURL)
		}
		if firstURL != "" {
			input.Media = append(input.Media, AliVideoMidia{Type: "first_frame", Url: firstURL})
		}
		if lastURL := strings.TrimSpace(input.LastFrameURL); lastURL != "" {
			input.Media = append(input.Media, AliVideoMidia{Type: "last_frame", Url: lastURL})
		}
		if audioURL := strings.TrimSpace(input.AudioURL); audioURL != "" {
			input.Media = append(input.Media, AliVideoMidia{Type: "reference_audio", Url: audioURL})
		}
	}

	// These legacy fields are not part of the Wan 3.0 input schema. A media
	// value above has already converted the compatible legacy fields.
	input.ImgURL = ""
	input.FirstFrameURL = ""
	input.LastFrameURL = ""
	input.AudioURL = ""
	if input.NegativePrompt != "" || input.Template != "" {
		return fmt.Errorf("%s does not support negative_prompt or template input fields", aliReq.Model)
	}
	input.NegativePrompt = ""
	input.Template = ""

	if strings.TrimSpace(input.Prompt) == "" && len(input.Media) == 0 {
		return fmt.Errorf("%s requires prompt or input.media", aliReq.Model)
	}

	counts := map[string]int{}
	for i := range input.Media {
		media := &input.Media[i]
		media.Type = strings.ToLower(strings.TrimSpace(media.Type))
		media.Url = strings.TrimSpace(media.Url)
		if !lo.Contains([]string{"first_frame", "last_frame", "reference_image", "reference_video", "reference_audio", "file", "link"}, media.Type) {
			return fmt.Errorf("%s does not support media type %q", aliReq.Model, media.Type)
		}
		if media.Url == "" {
			return fmt.Errorf("%s requires a non-empty URL for media type %s", aliReq.Model, media.Type)
		}
		counts[media.Type]++
	}

	if counts["first_frame"] > 1 || counts["last_frame"] > 1 {
		return fmt.Errorf("%s accepts at most one first_frame and one last_frame", aliReq.Model)
	}
	if counts["reference_image"] > 10 {
		return fmt.Errorf("%s accepts at most 10 reference_image inputs", aliReq.Model)
	}
	if counts["reference_video"] > 5 {
		return fmt.Errorf("%s accepts at most 5 reference_video inputs", aliReq.Model)
	}
	if counts["reference_audio"] > 5 {
		return fmt.Errorf("%s accepts at most 5 reference_audio inputs", aliReq.Model)
	}
	if counts["file"] > 1 || counts["link"] > 1 {
		return fmt.Errorf("%s accepts at most one file and one link input", aliReq.Model)
	}
	if (counts["file"] > 0 || counts["link"] > 0) && !boolParameterValue(aliReq.Parameters.PromptExtend, true) {
		return fmt.Errorf("%s requires prompt_extend=true when file or link media is provided", aliReq.Model)
	}

	frameCount := counts["first_frame"] + counts["last_frame"]
	referenceCount := counts["reference_image"] + counts["reference_video"] + counts["reference_audio"] + counts["file"] + counts["link"]
	if frameCount > 0 && referenceCount > 0 {
		return fmt.Errorf("%s cannot mix first_frame/last_frame with reference media", aliReq.Model)
	}
	if counts["file"] > 0 && counts["link"] > 0 {
		return fmt.Errorf("%s cannot mix file and link media", aliReq.Model)
	}
	if counts["last_frame"] > 0 && counts["first_frame"] == 0 {
		return fmt.Errorf("%s requires first_frame when last_frame is provided", aliReq.Model)
	}
	if len(input.Media) > 20 {
		return fmt.Errorf("%s accepts at most 20 media inputs", aliReq.Model)
	}
	return nil
}

func mergeAliInput(dst *AliVideoInput, src AliVideoInput) {
	if src.Prompt != "" {
		dst.Prompt = src.Prompt
	}
	if src.ImgURL != "" {
		dst.ImgURL = src.ImgURL
	}
	if src.FirstFrameURL != "" {
		dst.FirstFrameURL = src.FirstFrameURL
	}
	if src.LastFrameURL != "" {
		dst.LastFrameURL = src.LastFrameURL
	}
	if src.AudioURL != "" {
		dst.AudioURL = src.AudioURL
	}
	if src.NegativePrompt != "" {
		dst.NegativePrompt = src.NegativePrompt
	}
	if src.Template != "" {
		dst.Template = src.Template
	}
	if len(src.Media) > 0 {
		dst.Media = src.Media
	}
}

func mergeAliParameters(dst *AliVideoParameters, src AliVideoParameters) {
	if src.Resolution != "" {
		dst.Resolution = src.Resolution
	}
	if src.Size != "" {
		dst.Size = src.Size
	}
	if src.Ratio != "" {
		dst.Ratio = src.Ratio
	}
	if src.Duration != 0 {
		dst.Duration = src.Duration
	}
	if src.Seed != nil {
		dst.Seed = src.Seed
	}
	if src.Audio != nil {
		dst.Audio = src.Audio
	}
	// Pointer fields preserve explicit false values from nested metadata.
	if src.PromptExtend != nil {
		dst.PromptExtend = src.PromptExtend
	}
	if src.Watermark != nil {
		dst.Watermark = src.Watermark
	}
}

func applyNestedAliParameterOverrides(aliReq *AliVideoRequest, metadata map[string]interface{}) {
	if aliReq == nil || aliReq.Parameters == nil || metadata == nil {
		return
	}
	params, ok := metadata["parameters"].(map[string]interface{})
	if !ok {
		return
	}
	if value, exists := params["prompt_extend"]; exists {
		if enabled, ok := metadataBool(value); ok {
			aliReq.Parameters.PromptExtend = common.GetPointer(enabled)
		}
	}
	if value, exists := params["watermark"]; exists {
		if enabled, ok := metadataBool(value); ok {
			aliReq.Parameters.Watermark = common.GetPointer(enabled)
		}
	}
}

func metadataBool(value interface{}) (bool, bool) {
	switch v := value.(type) {
	case bool:
		return v, true
	case string:
		parsed, err := strconv.ParseBool(strings.TrimSpace(v))
		return parsed, err == nil
	default:
		return false, false
	}
}

func boolParameterValue(value *bool, defaultValue bool) bool {
	if value == nil {
		return defaultValue
	}
	return *value
}

func applyAliFlatMetadata(aliReq *AliVideoRequest, meta AliMetadata) {
	if meta.AudioURL != "" {
		aliReq.Input.AudioURL = meta.AudioURL
	}
	if meta.ImgURL != "" {
		aliReq.Input.ImgURL = meta.ImgURL
	}
	if meta.FirstFrameURL != "" {
		aliReq.Input.FirstFrameURL = meta.FirstFrameURL
	}
	if meta.LastFrameURL != "" {
		aliReq.Input.LastFrameURL = meta.LastFrameURL
	}
	if meta.NegativePrompt != "" {
		aliReq.Input.NegativePrompt = meta.NegativePrompt
	}
	if meta.Template != "" {
		aliReq.Input.Template = meta.Template
	}
	if len(meta.Media) > 0 {
		aliReq.Input.Media = meta.Media
	}
	if meta.Resolution != nil && *meta.Resolution != "" {
		aliReq.Parameters.Resolution = normalizeAliResolution(*meta.Resolution)
	}
	if meta.Size != nil && *meta.Size != "" {
		aliReq.Parameters.Size = *meta.Size
	}
	if meta.Ratio != nil && *meta.Ratio != "" {
		aliReq.Parameters.Ratio = *meta.Ratio
	}
	if meta.Duration != nil && (*meta.Duration > 0 || (isWan3Model(aliReq.Model) && *meta.Duration == -1)) {
		aliReq.Parameters.Duration = *meta.Duration
	}
	if meta.PromptExtend != nil {
		aliReq.Parameters.PromptExtend = meta.PromptExtend
	}
	if meta.Watermark != nil {
		aliReq.Parameters.Watermark = meta.Watermark
	}
	if meta.Audio != nil {
		aliReq.Parameters.Audio = meta.Audio
	}
	if meta.Seed != nil {
		aliReq.Parameters.Seed = meta.Seed
	}
}

func (a *TaskAdaptor) convertToAliRequest(info *relaycommon.RelayInfo, req relaycommon.TaskSubmitReq) (*AliVideoRequest, error) {
	upstreamModel := req.Model
	if info.IsModelMapped {
		upstreamModel = info.UpstreamModelName
	}
	aliReq := &AliVideoRequest{
		Model: upstreamModel,
		Input: AliVideoInput{
			Prompt: req.Prompt,
		},
		Parameters: &AliVideoParameters{
			PromptExtend: common.GetPointer(true),
			Watermark:    common.GetPointer(false),
		},
	}
	for _, media := range req.Media {
		if url := strings.TrimSpace(media.URL); url != "" {
			aliReq.Input.Media = append(aliReq.Input.Media, AliVideoMidia{
				Type: strings.TrimSpace(media.Type),
				Url:  url,
			})
		}
	}
	if isWan3Model(upstreamModel) {
		appendWan3ContentMedia(&aliReq.Input, req)
	}

	images := collectTaskImages(req)
	if err := applyTaskImages(aliReq, upstreamModel, images); err != nil {
		return nil, err
	}

	// size / resolution / ratio
	sizeValue := strings.TrimSpace(req.Size)
	if sizeValue == "" {
		sizeValue = strings.TrimSpace(req.Resolution)
	}
	if sizeValue != "" {
		switch {
		case isAliRatioFormat(sizeValue) || (isWan3Model(upstreamModel) && isWan3RatioFormat(sizeValue)):
			aliReq.Parameters.Ratio = sizeValue
		case isWan3Model(upstreamModel) && (strings.Contains(sizeValue, "*") || strings.Contains(sizeValue, "x") || strings.Contains(sizeValue, "X")):
			normalizedSize := strings.ReplaceAll(strings.ReplaceAll(sizeValue, "*", "x"), "X", "x")
			if ratio := aspectRatioFromPixelSize(normalizedSize); ratio != "" {
				aliReq.Parameters.Ratio = ratio
			} else {
				return nil, fmt.Errorf("invalid size: %s", sizeValue)
			}
		case strings.Contains(sizeValue, "*"):
			if strings.Contains(upstreamModel, "t2v") && !isHappyHorseModel(upstreamModel) {
				aliReq.Parameters.Size = sizeValue
			} else {
				if resolution, err := sizeToResolution(sizeValue); err == nil {
					aliReq.Parameters.Resolution = resolution
				} else {
					return nil, err
				}
			}
		case strings.Contains(sizeValue, "x") || strings.Contains(sizeValue, "X"):
			if aliUsesRatioParam(upstreamModel) {
				if ratio := aspectRatioFromPixelSize(sizeValue); ratio != "" {
					aliReq.Parameters.Ratio = ratio
				}
			} else if strings.Contains(upstreamModel, "t2v") {
				normalized := strings.ReplaceAll(strings.ReplaceAll(sizeValue, "x", "*"), "X", "*")
				aliReq.Parameters.Size = normalized
			} else {
				normalized := strings.ReplaceAll(strings.ReplaceAll(sizeValue, "x", "*"), "X", "*")
				if resolution, err := sizeToResolution(normalized); err == nil {
					aliReq.Parameters.Resolution = resolution
				}
			}
		default:
			aliReq.Parameters.Resolution = normalizeAliResolution(sizeValue)
		}
	} else if strings.TrimSpace(req.Resolution) != "" {
		aliReq.Parameters.Resolution = normalizeAliResolution(req.Resolution)
	} else {
		if isWan3Model(upstreamModel) {
			aliReq.Parameters.Resolution = "1080P"
			aliReq.Parameters.Ratio = "adaptive"
		} else if strings.Contains(upstreamModel, "t2v") {
			if aliUsesRatioParam(upstreamModel) {
				aliReq.Parameters.Ratio = "16:9"
				aliReq.Parameters.Resolution = "720P"
			} else if strings.HasPrefix(upstreamModel, "wan2.5") || strings.HasPrefix(upstreamModel, "wan2.2") {
				aliReq.Parameters.Size = "1920*1080"
			} else {
				aliReq.Parameters.Size = "1280*720"
			}
		} else {
			switch {
			case strings.HasPrefix(upstreamModel, "wan2.6"):
				aliReq.Parameters.Resolution = "1080P"
			case strings.HasPrefix(upstreamModel, "wan2.5"):
				aliReq.Parameters.Resolution = "1080P"
			case strings.HasPrefix(upstreamModel, "wan2.2-i2v-flash"):
				aliReq.Parameters.Resolution = "720P"
			case strings.HasPrefix(upstreamModel, "wan2.2-i2v-plus"):
				aliReq.Parameters.Resolution = "1080P"
			default:
				aliReq.Parameters.Resolution = "720P"
			}
		}
	}

	if req.Duration != 0 {
		aliReq.Parameters.Duration = req.Duration
	} else if req.Seconds != "" {
		seconds, err := strconv.Atoi(req.Seconds)
		if err != nil {
			return nil, errors.Wrap(err, "convert seconds to int failed")
		}
		aliReq.Parameters.Duration = seconds
	} else {
		aliReq.Parameters.Duration = 5
	}

	if req.Metadata != nil {
		var envelope aliMetadataEnvelope
		if err := taskcommon.UnmarshalMetadata(req.Metadata, &envelope); err != nil {
			return nil, errors.Wrap(err, "unmarshal metadata failed")
		}
		if envelope.Input != nil {
			mergeAliInput(&aliReq.Input, *envelope.Input)
		}
		if envelope.Parameters != nil {
			mergeAliParameters(aliReq.Parameters, *envelope.Parameters)
		}
		applyNestedAliParameterOverrides(aliReq, req.Metadata)
		applyAliFlatMetadata(aliReq, envelope.AliMetadata)
	}

	if aliReq.Parameters.Ratio != "" && aliReq.Parameters.Size != "" && aliUsesRatioParam(upstreamModel) {
		aliReq.Parameters.Size = ""
	}

	if len(aliReq.Input.Media) == 0 && len(images) > 0 {
		if err := applyTaskImages(aliReq, upstreamModel, images); err != nil {
			return nil, err
		}
	}

	if err := normalizeHappyHorseInput(aliReq); err != nil {
		return nil, err
	}
	if err := normalizeWan3Input(aliReq); err != nil {
		return nil, err
	}
	if err := finalizeWan3Parameters(aliReq); err != nil {
		return nil, err
	}

	finalizeHappyHorseParameters(req, aliReq)

	return aliReq, nil
}

func finalizeWan3Parameters(aliReq *AliVideoRequest) error {
	if !isWan3Model(aliReq.Model) {
		return nil
	}

	resolution := normalizeAliResolution(aliReq.Parameters.Resolution)
	if resolution == "" {
		resolution = "1080P"
	}
	if resolution != "480P" && resolution != "720P" && resolution != "1080P" {
		return fmt.Errorf("%s does not support resolution %q", aliReq.Model, aliReq.Parameters.Resolution)
	}
	aliReq.Parameters.Resolution = resolution

	ratio := strings.ToLower(strings.TrimSpace(aliReq.Parameters.Ratio))
	if ratio == "" {
		ratio = "adaptive"
	}
	if !isWan3RatioFormat(ratio) {
		return fmt.Errorf("%s does not support ratio %q", aliReq.Model, aliReq.Parameters.Ratio)
	}
	aliReq.Parameters.Ratio = ratio

	if aliReq.Parameters.Duration == 0 {
		aliReq.Parameters.Duration = 5
	}
	if aliReq.Parameters.Duration != -1 && (aliReq.Parameters.Duration < 2 || aliReq.Parameters.Duration > 30) {
		return fmt.Errorf("%s duration must be between 2 and 30 seconds or -1", aliReq.Model)
	}
	if aliReq.Parameters.Seed != nil && (*aliReq.Parameters.Seed < -1 || *aliReq.Parameters.Seed > 2147483647) {
		return fmt.Errorf("%s seed must be -1 or between 0 and 2147483647", aliReq.Model)
	}
	return nil
}

// AdjustBillingOnComplete settles Wan 3.0 against the duration reported by
// DashScope. For reference-video requests, usage.duration includes both input
// and output duration, matching the provider's billing definition.
func (a *TaskAdaptor) AdjustBillingOnComplete(task *model.Task, _ *relaycommon.TaskInfo) int {
	modelName := task.Properties.OriginModelName
	if bc := task.PrivateData.BillingContext; bc != nil && bc.OriginModelName != "" {
		modelName = bc.OriginModelName
	}
	priceModelName := modelName
	if !isWan3Model(priceModelName) && isWan3Model(task.Properties.UpstreamModelName) {
		priceModelName = task.Properties.UpstreamModelName
	}
	if !isWan3Model(priceModelName) {
		return 0
	}

	var response AliVideoResponse
	if err := common.Unmarshal(task.Data, &response); err != nil || response.Usage.Duration <= 0 {
		return 0
	}

	bc := task.PrivateData.BillingContext
	if bc == nil || bc.ModelPrice <= 0 || bc.GroupRatio <= 0 {
		return 0
	}
	resolutionRatio := 1.0
	for key, ratio := range bc.OtherRatios {
		if strings.HasPrefix(key, "resolution-") && ratio > 0 {
			resolutionRatio = ratio
			break
		}
	}

	return int(ratio_setting.ModelPriceToUSD(priceModelName, bc.ModelPrice) *
		common.QuotaPerUnit * bc.GroupRatio * response.Usage.Duration * resolutionRatio)
}

// finalizeHappyHorseParameters ensures happyhorse models always use parameters.resolution for upstream and billing.
func finalizeHappyHorseParameters(req relaycommon.TaskSubmitReq, aliReq *AliVideoRequest) {
	if !isHappyHorseModel(aliReq.Model) {
		return
	}

	resolution := strings.TrimSpace(req.Resolution)
	if resolution == "" {
		resolution = req.DashScopeParameterString("resolution")
	}
	if resolution == "" {
		resolution = strings.TrimSpace(aliReq.Parameters.Resolution)
	}
	if resolution != "" {
		aliReq.Parameters.Resolution = normalizeAliResolution(resolution)
	} else {
		aliReq.Parameters.Resolution = "720P"
	}

	if !aliUsesRatioParam(aliReq.Model) {
		return
	}

	ratio := strings.TrimSpace(aliReq.Parameters.Ratio)
	if ratio == "" {
		ratio = req.DashScopeParameterString("ratio")
	}
	if ratio == "" {
		if req.Metadata != nil {
			if metaRatio, ok := req.Metadata["ratio"].(string); ok {
				ratio = strings.TrimSpace(metaRatio)
			}
		}
	}
	if ratio == "" {
		ratio = "16:9"
	}
	if isAliRatioFormat(ratio) {
		aliReq.Parameters.Ratio = ratio
	}
	if aliReq.Parameters.Ratio != "" && aliReq.Parameters.Size != "" {
		aliReq.Parameters.Size = ""
	}
}

// EstimateBilling 根据用户请求参数计算 OtherRatios（时长、分辨率等）。
// 在 ValidateRequestAndSetAction 之后、价格计算之前调用。
func (a *TaskAdaptor) EstimateBilling(c *gin.Context, info *relaycommon.RelayInfo) map[string]float64 {
	taskReq, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return nil
	}

	aliReq, err := a.convertToAliRequest(info, taskReq)
	if err != nil {
		return nil
	}

	seconds := happyHorseBillableSeconds(aliReq, taskReq)
	if isWan3Model(aliReq.Model) && seconds < 0 {
		// Wan 3.0's -1 means smart duration. Pre-charge against the documented
		// 30-second maximum and settle to usage.duration when the task completes.
		seconds = 30
	}
	otherRatios := map[string]float64{"seconds": seconds}
	ratios, err := ProcessAliOtherRatios(aliReq, info.OriginModelName)
	if err != nil {
		return otherRatios
	}
	for k, v := range ratios {
		otherRatios[k] = v
	}
	return otherRatios
}

// DoRequest delegates to common helper
func (a *TaskAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	return channel.DoTaskApiRequest(a, c, info, requestBody)
}

// DoResponse handles upstream response
func (a *TaskAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (taskID string, taskData []byte, taskErr *dto.TaskError) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		taskErr = service.TaskErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
		return
	}
	_ = resp.Body.Close()

	// 解析阿里响应
	var aliResp AliVideoResponse
	if err := common.Unmarshal(responseBody, &aliResp); err != nil {
		taskErr = service.TaskErrorWrapper(errors.Wrapf(err, "body: %s", responseBody), "unmarshal_response_body_failed", http.StatusInternalServerError)
		return
	}

	// 检查错误
	if aliResp.Code != "" {
		taskErr = service.TaskErrorWrapper(fmt.Errorf("%s: %s", aliResp.Code, aliResp.Message), "ali_api_error", resp.StatusCode)
		return
	}

	if aliResp.Output.TaskID == "" {
		taskErr = service.TaskErrorWrapper(fmt.Errorf("task_id is empty"), "invalid_response", http.StatusInternalServerError)
		return
	}

	// 转换为 OpenAI 格式响应
	openAIResp := dto.NewOpenAIVideo()
	openAIResp.ID = info.PublicTaskID
	openAIResp.TaskID = info.PublicTaskID
	openAIResp.Model = c.GetString("model")
	if openAIResp.Model == "" && info != nil {
		openAIResp.Model = info.OriginModelName
	}
	openAIResp.Status = convertAliStatus(aliResp.Output.TaskStatus)
	openAIResp.CreatedAt = common.GetTimestamp()

	// 返回 OpenAI 格式
	c.JSON(http.StatusOK, openAIResp)

	return aliResp.Output.TaskID, responseBody, nil
}

// FetchTask 查询任务状态
func (a *TaskAdaptor) FetchTask(baseUrl, key string, body map[string]any, proxy string) (*http.Response, error) {
	taskID, ok := body["task_id"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid task_id")
	}

	uri := fmt.Sprintf("%s/api/v1/tasks/%s", baseUrl, taskID)

	req, err := http.NewRequest(http.MethodGet, uri, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+key)

	client, err := service.GetHttpClientWithProxy(proxy)
	if err != nil {
		return nil, fmt.Errorf("new proxy http client failed: %w", err)
	}
	return client.Do(req)
}

func (a *TaskAdaptor) GetModelList() []string {
	return ModelList
}

func (a *TaskAdaptor) GetChannelName() string {
	return ChannelName
}

// ParseTaskResult 解析任务结果
func (a *TaskAdaptor) ParseTaskResult(respBody []byte) (*relaycommon.TaskInfo, error) {
	var aliResp AliVideoResponse
	if err := common.Unmarshal(respBody, &aliResp); err != nil {
		return nil, errors.Wrap(err, "unmarshal task result failed")
	}

	taskResult := relaycommon.TaskInfo{
		Code: 0,
	}

	// 状态映射
	switch aliResp.Output.TaskStatus {
	case "PENDING":
		taskResult.Status = model.TaskStatusQueued
	case "RUNNING":
		taskResult.Status = model.TaskStatusInProgress
	case "SUCCEEDED":
		taskResult.Status = model.TaskStatusSuccess
		// 阿里直接返回视频URL，不需要额外的代理端点
		taskResult.Url = aliResp.Output.VideoURL
	case "FAILED", "CANCELED", "UNKNOWN":
		taskResult.Status = model.TaskStatusFailure
		if aliResp.Message != "" {
			taskResult.Reason = aliResp.Message
		} else if aliResp.Output.Message != "" {
			taskResult.Reason = fmt.Sprintf("task failed, code: %s , message: %s", aliResp.Output.Code, aliResp.Output.Message)
		} else {
			taskResult.Reason = "task failed"
		}
	default:
		taskResult.Status = model.TaskStatusQueued
	}

	return &taskResult, nil
}

func (a *TaskAdaptor) ConvertToOpenAIVideo(task *model.Task) ([]byte, error) {
	var aliResp AliVideoResponse
	if err := common.Unmarshal(task.Data, &aliResp); err != nil {
		return nil, errors.Wrap(err, "unmarshal ali response failed")
	}

	openAIResp := dto.NewOpenAIVideo()
	openAIResp.ID = task.TaskID
	openAIResp.Status = convertAliStatus(aliResp.Output.TaskStatus)
	openAIResp.Model = task.Properties.OriginModelName
	openAIResp.SetProgressStr(task.Progress)
	openAIResp.CreatedAt = task.CreatedAt
	openAIResp.CompletedAt = task.UpdatedAt

	// 设置视频URL — 优先使用响应体中的 video_url，fallback 到轮询时保存的 ResultURL
	videoURL := aliResp.Output.VideoURL
	if videoURL == "" {
		videoURL = task.GetResultURL()
	}
	openAIResp.SetMetadata("url", videoURL)

	// 错误处理
	if aliResp.Code != "" {
		openAIResp.Error = &dto.OpenAIVideoError{
			Code:    aliResp.Code,
			Message: aliResp.Message,
		}
	} else if aliResp.Output.Code != "" {
		openAIResp.Error = &dto.OpenAIVideoError{
			Code:    aliResp.Output.Code,
			Message: aliResp.Output.Message,
		}
	}

	return common.Marshal(openAIResp)
}

func convertAliStatus(aliStatus string) string {
	switch aliStatus {
	case "PENDING":
		return dto.VideoStatusQueued
	case "RUNNING":
		return dto.VideoStatusInProgress
	case "SUCCEEDED":
		return dto.VideoStatusCompleted
	case "FAILED", "CANCELED", "UNKNOWN":
		return dto.VideoStatusFailed
	default:
		return dto.VideoStatusUnknown
	}
}
