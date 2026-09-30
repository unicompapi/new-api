package ali

import (
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/require"
)

func TestHappyHorseResolutionRatios(t *testing.T) {
	r11 := happyHorseResolutionRatios("happyhorse-1.1-t2v")
	require.Equal(t, 1.0, r11["720P"])
	require.InDelta(t, 4.0/3.0, r11["1080P"], 0.0001)

	r10 := happyHorseResolutionRatios("happyhorse-1.0-i2v")
	require.Equal(t, 1.0, r10["720P"])
	require.InDelta(t, 16.0/9.0, r10["1080P"], 0.0001)
}

func TestProcessAliOtherRatiosHappyHorse(t *testing.T) {
	req := &AliVideoRequest{
		Model: "happyhorse-1.1-t2v",
		Parameters: &AliVideoParameters{
			Resolution: "720P",
			Duration:   5,
		},
	}
	ratios, err := ProcessAliOtherRatios(req, "happyhorse-1.1-t2v")
	require.NoError(t, err)
	require.Equal(t, 1.0, ratios["resolution-720P"])

	req.Parameters.Resolution = "1080P"
	ratios, err = ProcessAliOtherRatios(req, "happyhorse-1.1-t2v")
	require.NoError(t, err)
	require.InDelta(t, 4.0/3.0, ratios["resolution-1080P"], 0.0001)
}

func TestHappyHorseBillableSecondsVideoEdit(t *testing.T) {
	aliReq := &AliVideoRequest{
		Model: "happyhorse-1.0-video-edit",
		Parameters: &AliVideoParameters{
			Duration: 5,
		},
	}
	taskReq := relaycommon.TaskSubmitReq{
		Metadata: map[string]interface{}{
			"input_video_duration": 3,
		},
	}
	require.Equal(t, 8.0, happyHorseBillableSeconds(aliReq, taskReq))
}

func TestFinalizeHappyHorseParametersResolution(t *testing.T) {
	taskReq := relaycommon.TaskSubmitReq{
		Model: "happyhorse-1.0-video-edit",
		Metadata: map[string]interface{}{
			"parameters": map[string]interface{}{
				"resolution": "1080P",
			},
		},
	}
	taskReq.NormalizeTaskRequest()

	aliReq := &AliVideoRequest{
		Model:      taskReq.Model,
		Parameters: &AliVideoParameters{Duration: 5},
	}
	finalizeHappyHorseParameters(taskReq, aliReq)
	require.Equal(t, "1080P", aliReq.Parameters.Resolution)

	ratios, err := ProcessAliOtherRatios(aliReq, taskReq.Model)
	require.NoError(t, err)
	require.InDelta(t, 16.0/9.0, ratios["resolution-1080P"], 0.0001)
}

func TestFinalizeHappyHorseParametersDefaults(t *testing.T) {
	taskReq := relaycommon.TaskSubmitReq{Model: "happyhorse-1.1-i2v"}
	aliReq := &AliVideoRequest{
		Model:      taskReq.Model,
		Parameters: &AliVideoParameters{},
	}
	finalizeHappyHorseParameters(taskReq, aliReq)
	require.Equal(t, "720P", aliReq.Parameters.Resolution)
}

func TestHappyHorseOfficialMediaMapping(t *testing.T) {
	tests := []struct {
		name      string
		model     string
		images    []string
		mediaType string
	}{
		{name: "i2v first frame", model: "happyhorse-1.1-i2v", images: []string{"https://example.com/start.png"}, mediaType: "first_frame"},
		{name: "r2v reference image", model: "happyhorse-1.1-r2v", images: []string{"https://example.com/reference.png"}, mediaType: "reference_image"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := relaycommon.TaskSubmitReq{Model: tt.model, Prompt: "a video", Images: tt.images}
			info := &relaycommon.RelayInfo{OriginModelName: tt.model, ChannelMeta: &relaycommon.ChannelMeta{}}
			aliReq, err := (&TaskAdaptor{}).convertToAliRequest(info, req)
			require.NoError(t, err)
			require.Len(t, aliReq.Input.Media, 1)
			require.Equal(t, tt.mediaType, aliReq.Input.Media[0].Type)
			require.Equal(t, tt.images[0], aliReq.Input.Media[0].Url)
			require.Empty(t, aliReq.Input.ImgURL)
		})
	}
}

func TestHappyHorseT2VRejectsMedia(t *testing.T) {
	req := relaycommon.TaskSubmitReq{
		Model:  "happyhorse-1.1-t2v",
		Prompt: "a video",
		Images: []string{"https://example.com/image.png"},
	}
	info := &relaycommon.RelayInfo{OriginModelName: req.Model, ChannelMeta: &relaycommon.ChannelMeta{}}
	_, err := (&TaskAdaptor{}).convertToAliRequest(info, req)
	require.Error(t, err)
	require.Contains(t, err.Error(), "does not support image inputs")
}

func TestHappyHorseI2VRequiresMedia(t *testing.T) {
	req := relaycommon.TaskSubmitReq{Model: "happyhorse-1.1-i2v", Prompt: "a video"}
	info := &relaycommon.RelayInfo{OriginModelName: req.Model, ChannelMeta: &relaycommon.ChannelMeta{}}
	_, err := (&TaskAdaptor{}).convertToAliRequest(info, req)
	require.Error(t, err)
	require.Contains(t, err.Error(), "requires one first_frame image input")
}

func TestHappyHorseMetadataMediaIsPreserved(t *testing.T) {
	req := relaycommon.TaskSubmitReq{
		Model:  "happyhorse-1.1-r2v",
		Prompt: "a video using the reference",
		Metadata: map[string]interface{}{
			"input": map[string]interface{}{
				"media": []interface{}{
					map[string]interface{}{
						"type": "reference_image",
						"url":  "https://example.com/reference.png",
					},
				},
			},
		},
	}
	info := &relaycommon.RelayInfo{OriginModelName: req.Model, ChannelMeta: &relaycommon.ChannelMeta{}}
	aliReq, err := (&TaskAdaptor{}).convertToAliRequest(info, req)
	require.NoError(t, err)
	require.Len(t, aliReq.Input.Media, 1)
	require.Equal(t, "reference_image", aliReq.Input.Media[0].Type)
	require.Equal(t, "https://example.com/reference.png", aliReq.Input.Media[0].Url)
}

func TestHappyHorseMediaValidation(t *testing.T) {
	tests := []struct {
		name      string
		model     string
		media     []relaycommon.TaskMediaItem
		errString string
	}{
		{
			name:      "i2v rejects reference image type",
			model:     "happyhorse-1.1-i2v",
			media:     []relaycommon.TaskMediaItem{{Type: "reference_image", URL: "https://example.com/image.png"}},
			errString: "requires media type first_frame",
		},
		{
			name:      "i2v rejects empty URL",
			model:     "happyhorse-1.1-i2v",
			media:     []relaycommon.TaskMediaItem{{Type: "first_frame", URL: "  "}},
			errString: "requires one first_frame image input",
		},
		{
			name:      "r2v rejects first frame type",
			model:     "happyhorse-1.1-r2v",
			media:     []relaycommon.TaskMediaItem{{Type: "first_frame", URL: "https://example.com/image.png"}},
			errString: "requires media type reference_image",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := relaycommon.TaskSubmitReq{Model: tt.model, Prompt: "a video", Media: tt.media}
			info := &relaycommon.RelayInfo{OriginModelName: tt.model, ChannelMeta: &relaycommon.ChannelMeta{}}
			_, err := (&TaskAdaptor{}).convertToAliRequest(info, req)
			require.Error(t, err)
			require.Contains(t, err.Error(), tt.errString)
		})
	}
}

func TestHappyHorseR2VMediaLimit(t *testing.T) {
	media := make([]relaycommon.TaskMediaItem, 11)
	for i := range media {
		media[i] = relaycommon.TaskMediaItem{Type: "reference_image", URL: "https://example.com/image.png"}
	}
	req := relaycommon.TaskSubmitReq{Model: "happyhorse-1.1-r2v", Prompt: "a video", Media: media}
	info := &relaycommon.RelayInfo{OriginModelName: req.Model, ChannelMeta: &relaycommon.ChannelMeta{}}
	_, err := (&TaskAdaptor{}).convertToAliRequest(info, req)
	require.Error(t, err)
	require.Contains(t, err.Error(), "supports at most 10 reference_image inputs")
}
