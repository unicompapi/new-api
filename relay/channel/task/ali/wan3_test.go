package ali

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/stretchr/testify/require"
)

func wan3Info(model string) *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{OriginModelName: model, ChannelMeta: &relaycommon.ChannelMeta{}}
}

func TestWan3ModelsAreListed(t *testing.T) {
	var foundVideo, foundPrime bool
	for _, model := range ModelList {
		foundVideo = foundVideo || model == "wan3.0-video"
		foundPrime = foundPrime || model == "wan3.0-video-prime"
	}
	require.True(t, foundVideo)
	require.True(t, foundPrime)
}

func TestWan3TextToVideoDefaults(t *testing.T) {
	req := relaycommon.TaskSubmitReq{Model: "wan3.0-video", Prompt: "a cat running"}
	converted, err := (&TaskAdaptor{}).convertToAliRequest(wan3Info(req.Model), req)
	require.NoError(t, err)
	require.Equal(t, "wan3.0-video", converted.Model)
	require.Empty(t, converted.Input.Media)
	require.Equal(t, "1080P", converted.Parameters.Resolution)
	require.Equal(t, "adaptive", converted.Parameters.Ratio)
	require.Equal(t, 5, converted.Parameters.Duration)
}

func TestWan3ResolutionBillingRatios(t *testing.T) {
	for _, tc := range []struct {
		resolution string
		want       float64
	}{
		{resolution: "480P", want: 0.5},
		{resolution: "720P", want: 1},
		{resolution: "1080P", want: 2},
	} {
		req := &AliVideoRequest{Model: "wan3.0-video", Parameters: &AliVideoParameters{Resolution: tc.resolution}}
		ratios, err := ProcessAliOtherRatios(req, req.Model)
		require.NoError(t, err)
		require.Equal(t, tc.want, ratios["resolution-"+tc.resolution])
	}
}

func TestWan3NestedBooleanParametersPreserveExplicitValues(t *testing.T) {
	req := relaycommon.TaskSubmitReq{
		Model:  "wan3.0-video",
		Prompt: "a scene",
		Metadata: map[string]interface{}{
			"parameters": map[string]interface{}{
				"prompt_extend": false,
				"watermark":     true,
			},
		},
	}
	converted, err := (&TaskAdaptor{}).convertToAliRequest(wan3Info(req.Model), req)
	require.NoError(t, err)
	require.NotNil(t, converted.Parameters.PromptExtend)
	require.False(t, *converted.Parameters.PromptExtend)
	require.NotNil(t, converted.Parameters.Watermark)
	require.True(t, *converted.Parameters.Watermark)
}

func TestWan3SmartDurationUsesMaximumForPrebilling(t *testing.T) {
	req := relaycommon.TaskSubmitReq{
		Model:  "wan3.0-video",
		Prompt: "a long scene",
		Metadata: map[string]interface{}{
			"duration": -1,
		},
	}
	converted, err := (&TaskAdaptor{}).convertToAliRequest(wan3Info(req.Model), req)
	require.NoError(t, err)
	require.Equal(t, -1, converted.Parameters.Duration)
}

func TestWan3CompletionBillingUsesUsageDuration(t *testing.T) {
	task := &model.Task{
		Properties: model.Properties{OriginModelName: "wan3.0-video"},
		Data:       []byte(`{"usage":{"duration":8}}`),
		PrivateData: model.TaskPrivateData{BillingContext: &model.TaskBillingContext{
			OriginModelName: "wan3.0-video",
			ModelPrice:      0.6,
			GroupRatio:      1,
			OtherRatios:     map[string]float64{"seconds": 30, "resolution-1080P": 2},
		}},
	}
	quota := (&TaskAdaptor{}).AdjustBillingOnComplete(task, nil)
	want := int(ratio_setting.ModelPriceToUSD("wan3.0-video", 0.6) * common.QuotaPerUnit * 8 * 2)
	require.Equal(t, want, quota)
}

func TestWan3FirstAndLastFrameMapping(t *testing.T) {
	req := relaycommon.TaskSubmitReq{
		Model:  "wan3.0-video-prime",
		Prompt: "transition between the frames",
		Images: []string{"https://example.com/first.png", "https://example.com/last.png"},
	}
	converted, err := (&TaskAdaptor{}).convertToAliRequest(wan3Info(req.Model), req)
	require.NoError(t, err)
	require.Equal(t, []AliVideoMidia{
		{Type: "first_frame", Url: "https://example.com/first.png"},
		{Type: "last_frame", Url: "https://example.com/last.png"},
	}, converted.Input.Media)
}

func TestWan3ImageBatchMapsToReferenceImages(t *testing.T) {
	req := relaycommon.TaskSubmitReq{
		Model:  "wan3.0-video",
		Prompt: "use the reference images for character consistency",
		Images: []string{
			"https://example.com/one.png",
			"https://example.com/two.png",
			"https://example.com/three.png",
		},
	}
	converted, err := (&TaskAdaptor{}).convertToAliRequest(wan3Info(req.Model), req)
	require.NoError(t, err)
	require.Len(t, converted.Input.Media, 3)
	for _, media := range converted.Input.Media {
		require.Equal(t, "reference_image", media.Type)
	}
}

func TestWan3ReferenceMediaPreservesTypes(t *testing.T) {
	req := relaycommon.TaskSubmitReq{
		Model:  "wan3.0-video",
		Prompt: "use image 1, video 1 and audio 1 as references",
		Media: []relaycommon.TaskMediaItem{
			{Type: "reference_image", URL: "https://example.com/ref.png"},
			{Type: "reference_video", URL: "https://example.com/ref.mp4"},
			{Type: "reference_audio", URL: "https://example.com/ref.mp3"},
		},
	}
	converted, err := (&TaskAdaptor{}).convertToAliRequest(wan3Info(req.Model), req)
	require.NoError(t, err)
	require.Equal(t, "reference_image", converted.Input.Media[0].Type)
	require.Equal(t, "reference_video", converted.Input.Media[1].Type)
	require.Equal(t, "reference_audio", converted.Input.Media[2].Type)
}

func TestWan3MetadataContentMediaIsConverted(t *testing.T) {
	req := relaycommon.TaskSubmitReq{
		Model:  "wan3.0-video",
		Prompt: "use the reference images",
		Metadata: map[string]interface{}{
			"content": []interface{}{
				map[string]interface{}{
					"type": "image_url",
					"image_url": map[string]interface{}{
						"url": "https://example.com/reference.png",
					},
				},
			},
		},
	}
	converted, err := (&TaskAdaptor{}).convertToAliRequest(wan3Info(req.Model), req)
	require.NoError(t, err)
	require.Equal(t, []AliVideoMidia{{Type: "first_frame", Url: "https://example.com/reference.png"}}, converted.Input.Media)
}

func TestWan3AcceptsAdaptiveAndSmartDuration(t *testing.T) {
	duration := -1
	req := relaycommon.TaskSubmitReq{
		Model:  "wan3.0-video",
		Prompt: "an adaptive scene",
		Metadata: map[string]interface{}{
			"parameters": map[string]interface{}{
				"ratio":    "adaptive",
				"duration": duration,
			},
		},
	}
	converted, err := (&TaskAdaptor{}).convertToAliRequest(wan3Info(req.Model), req)
	require.NoError(t, err)
	require.Equal(t, "adaptive", converted.Parameters.Ratio)
	require.Equal(t, -1, converted.Parameters.Duration)
}

func TestWan3PreservesExplicitZeroSeed(t *testing.T) {
	seed := 0
	req := relaycommon.TaskSubmitReq{
		Model:  "wan3.0-video",
		Prompt: "repeatable seed",
		Metadata: map[string]interface{}{
			"seed": seed,
		},
	}
	converted, err := (&TaskAdaptor{}).convertToAliRequest(wan3Info(req.Model), req)
	require.NoError(t, err)
	require.NotNil(t, converted.Parameters.Seed)
	require.Equal(t, 0, *converted.Parameters.Seed)
}

func TestWan3RejectsInvalidMediaCombination(t *testing.T) {
	req := relaycommon.TaskSubmitReq{
		Model:  "wan3.0-video",
		Prompt: "invalid mixed input",
		Media: []relaycommon.TaskMediaItem{
			{Type: "first_frame", URL: "https://example.com/first.png"},
			{Type: "reference_image", URL: "https://example.com/reference.png"},
		},
	}
	_, err := (&TaskAdaptor{}).convertToAliRequest(wan3Info(req.Model), req)
	require.Error(t, err)
	require.Contains(t, err.Error(), "cannot mix first_frame/last_frame")
}

func TestWan3FileMediaRequiresPromptExtend(t *testing.T) {
	req := relaycommon.TaskSubmitReq{
		Model:  "wan3.0-video",
		Prompt: "make a product video from this document",
		Media:  []relaycommon.TaskMediaItem{{Type: "file", URL: "https://example.com/product.pdf"}},
		Metadata: map[string]interface{}{
			"prompt_extend": false,
		},
	}
	_, err := (&TaskAdaptor{}).convertToAliRequest(wan3Info(req.Model), req)
	require.Error(t, err)
	require.Contains(t, err.Error(), "requires prompt_extend=true")
}

func TestWan3RejectsInvalidParameters(t *testing.T) {
	for _, tc := range []struct {
		name     string
		metadata map[string]interface{}
		want     string
	}{
		{
			name: "duration",
			metadata: map[string]interface{}{
				"duration": 31,
			},
			want: "duration must be between",
		},
		{
			name: "ratio",
			metadata: map[string]interface{}{
				"ratio": "2:1",
			},
			want: "does not support ratio",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := relaycommon.TaskSubmitReq{Model: "wan3.0-video", Prompt: "test", Metadata: tc.metadata}
			_, err := (&TaskAdaptor{}).convertToAliRequest(wan3Info(req.Model), req)
			require.Error(t, err)
			require.Contains(t, err.Error(), tc.want)
		})
	}
}
