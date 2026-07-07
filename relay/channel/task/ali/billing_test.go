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
