package tokenpony

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConvertRequestPreservesSeedanceMediaAndOptionalValues(t *testing.T) {
	req := relaycommon.TaskSubmitReq{
		Prompt:     "continue the shot",
		Model:      ModelSeedance25,
		Resolution: "720P",
		Duration:   8,
		Content: []relaycommon.TaskContentItem{
			{Type: "image_url", Role: "first_frame", ImageURL: &relaycommon.TaskMediaURL{URL: "https://cdn.example/start.jpg"}},
			{Type: "image_url", Role: "last_frame", ImageURL: &relaycommon.TaskMediaURL{URL: "https://cdn.example/end.jpg"}},
			{Type: "video_url", VideoURL: &relaycommon.TaskMediaURL{URL: "https://cdn.example/ref.mp4"}},
			{Type: "audio_url", AudioURL: &relaycommon.TaskMediaURL{URL: "asset://voice_1"}},
		},
		Metadata: map[string]any{
			"ratio":             "adaptive",
			"generate_audio":    false,
			"watermark":         false,
			"seed":              0,
			"return_last_frame": true,
		},
	}

	payload, err := convertRequest(&req, req.Model)
	require.NoError(t, err)
	assert.Equal(t, "720P", payload.Resolution)
	require.NotNil(t, payload.GenerateAudio)
	assert.False(t, bool(*payload.GenerateAudio))
	require.NotNil(t, payload.Watermark)
	assert.False(t, bool(*payload.Watermark))
	require.NotNil(t, payload.Seed)
	assert.Zero(t, int(*payload.Seed))
	require.NotNil(t, payload.ReturnLastFrame)
	assert.True(t, bool(*payload.ReturnLastFrame))
	assert.Equal(t, []string{"first_image", "last_image", "reference_video", "reference_audio"}, []string{
		payload.Media[0].Type, payload.Media[1].Type, payload.Media[2].Type, payload.Media[3].Type,
	})
}

func TestUnifiedFrameRetryJSONMapsFirstFrameToFirstImage(t *testing.T) {
	var req relaycommon.TaskSubmitReq
	require.NoError(t, common.Unmarshal([]byte(`{
		"model":"doubao-seedance-2-0-260128",
		"prompt":"retry shot",
		"resolution":"720P",
		"ratio":"16:9",
		"duration":5,
		"generate_audio":true,
		"content":[
			{"type":"text","text":"retry shot"},
			{"type":"image_url","role":"first_frame","image_url":{"url":"https://cdn.example/input.png"}}
		]
	}`), &req))
	payload, err := convertRequest(&req, req.Model)
	require.NoError(t, err)
	require.Len(t, payload.Media, 1)
	assert.Equal(t, "first_image", payload.Media[0].Type)
	assert.NotEqual(t, "reference_image", payload.Media[0].Type)
	require.NotNil(t, payload.GenerateAudio)
	assert.True(t, bool(*payload.GenerateAudio))
}

func TestValidateRequestModelBoundaries(t *testing.T) {
	valid := func(modelName string) requestPayload {
		d := dto.IntValue(4)
		return requestPayload{Model: modelName, Prompt: "test", Resolution: "480P", Ratio: "16:9", Duration: &d}
	}
	tests := []struct {
		name    string
		mutate  func(*requestPayload)
		wantErr string
	}{
		{"2.0 max duration", func(p *requestPayload) { d := dto.IntValue(15); p.Duration = &d }, ""},
		{"2.0 over duration", func(p *requestPayload) { d := dto.IntValue(16); p.Duration = &d }, "between 4 and 15"},
		{"2.0 pure audio", func(p *requestPayload) {
			p.Media = []relaycommon.TaskMediaItem{{Type: "reference_audio", URL: "https://cdn.example/a.mp3"}}
		}, "requires at least one"},
		{"2.0 first frame arbitrary ratio", func(p *requestPayload) {
			p.Media = []relaycommon.TaskMediaItem{{Type: "first_image", URL: "https://cdn.example/a.jpg"}}
		}, ""},
		{"lowercase resolution", func(p *requestPayload) { p.Resolution = "720p" }, ""},
		{"invalid resolution", func(p *requestPayload) { p.Resolution = "2K" }, "invalid resolution"},
		{"invalid asset id", func(p *requestPayload) {
			p.Media = []relaycommon.TaskMediaItem{{Type: "reference_image", URL: "asset://bad id"}}
		}, "url must be"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			payload := valid(ModelSeedance20)
			tc.mutate(&payload)
			err := validateRequest(&payload)
			if tc.wantErr == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tc.wantErr)
			}
		})
	}

	t.Run("2.5 requires adaptive for boundary frames", func(t *testing.T) {
		payload := valid(ModelSeedance25)
		payload.Media = []relaycommon.TaskMediaItem{{Type: "last_image", URL: "https://cdn.example/end.jpg"}}
		require.ErrorContains(t, validateRequest(&payload), "ratio adaptive")
		payload.Ratio = "adaptive"
		d := dto.IntValue(30)
		payload.Duration = &d
		require.NoError(t, validateRequest(&payload))
	})

	t.Run("2.5 rejects service tier", func(t *testing.T) {
		payload := valid(ModelSeedance25)
		payload.ServiceTier = "default"
		require.ErrorContains(t, validateRequest(&payload), "only supported by Seedance 2.0")
	})

	t.Run("model-specific media counts", func(t *testing.T) {
		payload := valid(ModelSeedance20)
		for i := 0; i < 10; i++ {
			payload.Media = append(payload.Media, relaycommon.TaskMediaItem{Type: "reference_image", URL: "https://cdn.example/a.jpg"})
		}
		require.ErrorContains(t, validateRequest(&payload), "images 10/9")
		payload.Model = ModelSeedance25
		require.NoError(t, validateRequest(&payload))
	})
}

func TestParseEnvelopeBusinessErrors(t *testing.T) {
	_, err := parseEnvelope([]byte(`{"code":10108,"message":"bad params","data":{}}`))
	require.NotNil(t, err)
	assert.Equal(t, http.StatusBadRequest, err.StatusCode)
	assert.True(t, err.LocalError)

	_, err = parseEnvelope([]byte(`{"code":10107,"message":"missing","data":{}}`))
	require.NotNil(t, err)
	assert.Equal(t, http.StatusNotFound, err.StatusCode)

	_, err = parseEnvelope([]byte(`{"code":19999,"message":"unknown","data":{}}`))
	require.NotNil(t, err)
	assert.Equal(t, http.StatusBadGateway, err.StatusCode)
}

func TestDoResponseUsesDataID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	resp := &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"code":200,"data":{"id":"upstream-1"}}`))}
	info := &relaycommon.RelayInfo{OriginModelName: ModelSeedance20, TaskRelayInfo: &relaycommon.TaskRelayInfo{PublicTaskID: "task_public"}}

	id, _, taskErr := (&TaskAdaptor{}).DoResponse(ctx, resp, info)
	require.Nil(t, taskErr)
	assert.Equal(t, "upstream-1", id)
	assert.Equal(t, http.StatusOK, recorder.Code)

	resp = &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"code":200,"data":{}}`))}
	_, _, taskErr = (&TaskAdaptor{}).DoResponse(ctx, resp, info)
	require.NotNil(t, taskErr)
	assert.Equal(t, http.StatusBadGateway, taskErr.StatusCode)
}

func TestParseTaskLifecycleAndResultVariants(t *testing.T) {
	adaptor := &TaskAdaptor{}
	tests := []struct {
		name       string
		data       string
		status     model.TaskStatus
		url        string
		lastFrame  string
		tokens     int
		reasonPart string
	}{
		{"pending", `{"task_id":"u1","task_status":"PENDING"}`, model.TaskStatusQueued, "", "", 0, ""},
		{"running", `{"task_id":"u1","task_status":"RUNNING"}`, model.TaskStatusInProgress, "", "", 0, ""},
		{"output string", `{"task_id":"u1","task_status":"COMPLETED","result":{"output":"https://cdn.example/v.mp4"},"usage":{"total_tokens":123,"completion_tokens":120}}`, model.TaskStatusSuccess, "https://cdn.example/v.mp4", "", 123, ""},
		{"output object", `{"task_id":"u1","task_status":"COMPLETED","result":{"output":{"video_url":"https://cdn.example/v.mp4","last_frame_url":"https://cdn.example/f.jpg"}},"usage":{"total_tokens":7}}`, model.TaskStatusSuccess, "https://cdn.example/v.mp4", "https://cdn.example/f.jpg", 7, ""},
		{"legacy result", `{"task_id":"u1","task_status":"COMPLETED","result":{"result":"https://cdn.example/legacy.mp4"},"usage":{"total_tokens":9}}`, model.TaskStatusSuccess, "https://cdn.example/legacy.mp4", "", 9, ""},
		{"missing output", `{"task_id":"u1","task_status":"COMPLETED","result":{}}`, model.TaskStatusFailure, "", "", 0, "missing output"},
		{"canceled", `{"task_id":"u1","task_status":"CANCELED"}`, model.TaskStatusFailure, "", "", 0, "canceled"},
		{"failed detail", `{"task_id":"u1","task_status":"FAILED","task_status_msg":"provider failed","detail":{"message":"safety rejection"}}`, model.TaskStatusFailure, "", "", 0, "safety rejection"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			body := []byte(`{"code":200,"data":` + tc.data + `}`)
			result, err := adaptor.ParseTaskResult(body)
			require.NoError(t, err)
			assert.Equal(t, string(tc.status), result.Status)
			assert.Equal(t, tc.url, result.Url)
			assert.Equal(t, tc.lastFrame, result.LastFrameURL)
			assert.Equal(t, tc.tokens, result.TotalTokens)
			assert.Contains(t, result.Reason, tc.reasonPart)
		})
	}

	result, err := adaptor.ParseTaskResult([]byte(`{"code":10107,"message":"not found","data":{}}`))
	require.NoError(t, err)
	assert.Equal(t, string(model.TaskStatusFailure), result.Status)
	assert.Contains(t, result.Reason, "10107")

	_, err = adaptor.ParseTaskResult([]byte(`{"code":200,"data":{"task_status":"PAUSED"}}`))
	require.ErrorContains(t, err, "unknown TokenPony task status")

	_, err = adaptor.ParseTaskResult([]byte(`{"code":200,"data":{"task_status":"COMPLETED","result":{"output":"https://cdn.example/v.mp4"}}}`))
	require.ErrorContains(t, err, "missing authoritative usage.total_tokens")
}

func TestFetchTaskUsesDocumentedPathAndIncludeDetail(t *testing.T) {
	service.InitHttpClient()
	var path, query, auth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path, query, auth = r.URL.Path, r.URL.RawQuery, r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":200,"data":{"task_status":"RUNNING"}}`))
	}))
	defer server.Close()

	adaptor := &TaskAdaptor{httpTimeoutSecond: 2}
	resp, err := adaptor.FetchTask(server.URL, "secret", map[string]any{"task_id": "id/with/slash"}, "")
	require.NoError(t, err)
	_ = resp.Body.Close()
	assert.Equal(t, "/v1/aigc/tasks/id/with/slash", path)
	assert.Equal(t, "include_detail=true", query)
	assert.Equal(t, "Bearer secret", auth)
}

func TestBuildRequestBodyRejectsFixedPriceBilling(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Set("task_request", relaycommon.TaskSubmitReq{Prompt: "x", Model: ModelSeedance20})
	adaptor := &TaskAdaptor{}
	info := &relaycommon.RelayInfo{PriceData: types.PriceData{UsePrice: true}}
	_, err := adaptor.BuildRequestBody(ctx, info)
	require.ErrorContains(t, err, "requires a positive model multiplier")
}

func TestUsageBillingUsesFrozenTaskRatios(t *testing.T) {
	adaptor := &TaskAdaptor{}
	task := &model.Task{PrivateData: model.TaskPrivateData{BillingContext: &model.TaskBillingContext{ModelRatio: 1.25, GroupRatio: 2}}}
	assert.Equal(t, 250, adaptor.AdjustBillingOnComplete(task, &relaycommon.TaskInfo{TotalTokens: 100}))
	assert.Zero(t, adaptor.AdjustBillingOnComplete(task, &relaycommon.TaskInfo{}))
}

func TestAssetReferencesRequireActiveGetAssetResult(t *testing.T) {
	service.InitHttpClient()
	status := "Active"
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		assert.Equal(t, "/v1/assets/GetAsset", r.URL.Path)
		assert.Equal(t, "Bearer fixture-key", r.Header.Get("Authorization"))
		body, _ := io.ReadAll(r.Body)
		assert.JSONEq(t, `{"Id":"asset-1"}`, string(body))
		_, _ = w.Write([]byte(`{"ResponseMetadata":{},"Result":{"Id":"asset-1","Status":"` + status + `"}}`))
	}))
	defer server.Close()

	adaptor := &TaskAdaptor{apiKey: "fixture-key", baseURL: server.URL, httpTimeoutSecond: 2}
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}}
	payload := &requestPayload{Media: []relaycommon.TaskMediaItem{
		{Type: "reference_image", URL: "asset://asset-1"},
		{Type: "first_image", URL: "asset://asset-1"},
	}}
	require.NoError(t, adaptor.ensureAssetsActive(context.Background(), info, payload))
	assert.Equal(t, 1, requests, "duplicate asset references should be queried once")

	status = "Processing"
	err := adaptor.ensureAssetsActive(context.Background(), info, payload)
	require.ErrorContains(t, err, "is not Active")
}

func TestBuildRequestSerializesDocumentedCreatePayload(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", nil)
	req := relaycommon.TaskSubmitReq{Prompt: "x", Model: ModelSeedance20, Metadata: map[string]any{"resolution": "1080P", "ratio": "1:1"}}
	ctx.Set("task_request", req)
	info := &relaycommon.RelayInfo{
		OriginModelName: ModelSeedance20,
		PriceData:       types.PriceData{ModelRatio: 1},
		ChannelMeta:     &relaycommon.ChannelMeta{},
	}
	body, err := (&TaskAdaptor{}).BuildRequestBody(ctx, info)
	require.NoError(t, err)
	data, err := io.ReadAll(body)
	require.NoError(t, err)
	assert.True(t, bytes.Contains(data, []byte(`"model":"doubao-seedance-2-0-260128"`)))
	assert.True(t, bytes.Contains(data, []byte(`"resolution":"1080P"`)))
}

func TestOpenAIVideosMultipartTextAndSingleReferenceReachTokenPonyPayload(t *testing.T) {
	oldDir := constant.TaskMediaDir
	oldAddress := system_setting.ServerAddress
	constant.TaskMediaDir = t.TempDir()
	system_setting.ServerAddress = "https://gateway.example"
	t.Cleanup(func() {
		constant.TaskMediaDir = oldDir
		system_setting.ServerAddress = oldAddress
	})

	t.Run("text to video", func(t *testing.T) {
		ctx := multipartVideoContext(t, map[string]string{
			"model": ModelSeedance20, "prompt": "clouds moving", "seconds": "4", "size": "1280x720",
		}, nil)
		info := tokenPonyTestRelayInfo()
		require.Nil(t, (&TaskAdaptor{}).ValidateRequestAndSetAction(ctx, info))
		body, err := (&TaskAdaptor{}).BuildRequestBody(ctx, info)
		require.NoError(t, err)
		var payload requestPayload
		require.NoError(t, common.DecodeJson(body, &payload))
		assert.Equal(t, "16:9", payload.Ratio)
		assert.Equal(t, "720P", payload.Resolution)
		assert.Empty(t, payload.Media)
	})

	t.Run("single input reference", func(t *testing.T) {
		ctx := multipartVideoContext(t, map[string]string{
			"model": ModelSeedance25, "prompt": "animate the reference", "seconds": "4", "size": "720x1280",
		}, [][]byte{[]byte("\x89PNG\r\n\x1a\nfixture")})
		info := tokenPonyTestRelayInfo()
		require.Nil(t, (&TaskAdaptor{}).ValidateRequestAndSetAction(ctx, info))
		t.Cleanup(func() { service.RemoveTaskInputMedia(info.InputMediaFile) })

		body, err := (&TaskAdaptor{}).BuildRequestBody(ctx, info)
		require.NoError(t, err)
		var payload requestPayload
		require.NoError(t, common.DecodeJson(body, &payload))
		require.Len(t, payload.Media, 1)
		assert.Equal(t, "reference_image", payload.Media[0].Type)
		assert.True(t, strings.HasPrefix(payload.Media[0].URL, "https://gateway.example/v1/video-inputs/"))
		assert.Equal(t, "9:16", payload.Ratio)
		assert.Equal(t, "720P", payload.Resolution)
	})
}

func TestOpenAIVideosMultipartRejectsUnsafeInputReference(t *testing.T) {
	oldDir := constant.TaskMediaDir
	oldAddress := system_setting.ServerAddress
	constant.TaskMediaDir = t.TempDir()
	system_setting.ServerAddress = "https://gateway.example"
	t.Cleanup(func() {
		constant.TaskMediaDir = oldDir
		system_setting.ServerAddress = oldAddress
	})
	fields := map[string]string{"model": ModelSeedance20, "prompt": "test", "seconds": "4"}

	t.Run("multiple files", func(t *testing.T) {
		ctx := multipartVideoContext(t, fields, [][]byte{[]byte("\x89PNG\r\n\x1a\none"), []byte("\x89PNG\r\n\x1a\ntwo")})
		taskErr := (&TaskAdaptor{}).ValidateRequestAndSetAction(ctx, tokenPonyTestRelayInfo())
		require.NotNil(t, taskErr)
		assert.Equal(t, http.StatusBadRequest, taskErr.StatusCode)
		assert.Contains(t, taskErr.Message, "exactly one")
	})

	t.Run("empty file", func(t *testing.T) {
		ctx := multipartVideoContext(t, fields, [][]byte{{}})
		taskErr := (&TaskAdaptor{}).ValidateRequestAndSetAction(ctx, tokenPonyTestRelayInfo())
		require.NotNil(t, taskErr)
		assert.Contains(t, taskErr.Message, "must not be empty")
	})

	t.Run("wrong MIME", func(t *testing.T) {
		ctx := multipartVideoContext(t, fields, [][]byte{[]byte("plain text")})
		taskErr := (&TaskAdaptor{}).ValidateRequestAndSetAction(ctx, tokenPonyTestRelayInfo())
		require.NotNil(t, taskErr)
		assert.Contains(t, taskErr.Message, "unsupported MIME type")
	})

	t.Run("oversized file", func(t *testing.T) {
		ctx := multipartVideoContext(t, fields, [][]byte{bytes.Repeat([]byte{'x'}, (20<<20)+1)})
		taskErr := (&TaskAdaptor{}).ValidateRequestAndSetAction(ctx, tokenPonyTestRelayInfo())
		require.NotNil(t, taskErr)
		assert.Contains(t, taskErr.Message, "20 MB limit")
	})

	t.Run("unreachable public address", func(t *testing.T) {
		system_setting.ServerAddress = "http://127.0.0.1:3000"
		defer func() { system_setting.ServerAddress = "https://gateway.example" }()
		ctx := multipartVideoContext(t, fields, [][]byte{[]byte("\x89PNG\r\n\x1a\nfixture")})
		taskErr := (&TaskAdaptor{}).ValidateRequestAndSetAction(ctx, tokenPonyTestRelayInfo())
		require.NotNil(t, taskErr)
		assert.Contains(t, taskErr.Message, "public HTTPS")
	})
}

func TestOpenAISizeMappingAndResolutionNormalization(t *testing.T) {
	tests := []struct{ size, ratio, resolution string }{
		{"1280x720", "16:9", "720P"},
		{"720x1280", "9:16", "720P"},
		{"1920x1080", "16:9", "1080P"},
		{"1080x1920", "9:16", "1080P"},
	}
	for _, tc := range tests {
		t.Run(tc.size, func(t *testing.T) {
			payload := &requestPayload{Model: ModelSeedance20, Prompt: "test"}
			require.NoError(t, applyOpenAISize(payload, tc.size))
			assert.Equal(t, tc.ratio, payload.Ratio)
			assert.Equal(t, tc.resolution, payload.Resolution)
		})
	}
	for _, value := range []string{"720p", "720P"} {
		resolution, err := normalizeResolution(value)
		require.NoError(t, err)
		assert.Equal(t, "720P", resolution)
	}
	require.ErrorContains(t, applyOpenAISize(&requestPayload{}, "1024x1792"), "unsupported size")
	require.ErrorContains(t, applyOpenAISize(&requestPayload{Resolution: "1080P"}, "1280x720"), "conflicts")
	_, err := normalizeResolution("1440p")
	require.ErrorContains(t, err, "invalid resolution")
}

func TestTaskInitPersistsInputMediaPathForRecovery(t *testing.T) {
	info := tokenPonyTestRelayInfo()
	info.InputMediaFile = "inputs/abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMN"
	task := model.InitTask("tokenpony", info)
	assert.Equal(t, info.InputMediaFile, task.PrivateData.InputMediaFile)
}

func tokenPonyTestRelayInfo() *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{
		TaskRelayInfo: &relaycommon.TaskRelayInfo{},
		PriceData:     types.PriceData{ModelRatio: 1},
		ChannelMeta:   &relaycommon.ChannelMeta{},
	}
}

func multipartVideoContext(t *testing.T, fields map[string]string, files [][]byte) *gin.Context {
	t.Helper()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	for key, value := range fields {
		require.NoError(t, w.WriteField(key, value))
	}
	for i, data := range files {
		part, err := w.CreateFormFile("input_reference", fmt.Sprintf("reference-%d.png", i))
		require.NoError(t, err)
		_, err = part.Write(data)
		require.NoError(t, err)
	}
	require.NoError(t, w.Close())
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", &body)
	ctx.Request.Header.Set("Content-Type", w.FormDataContentType())
	storage, err := common.GetBodyStorage(ctx)
	require.NoError(t, err)
	ctx.Request.Body = io.NopCloser(storage)
	t.Cleanup(func() { common.CleanupBodyStorage(ctx) })
	return ctx
}
