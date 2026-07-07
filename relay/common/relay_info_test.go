package common

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/require"
)

func TestRelayInfoGetFinalRequestRelayFormatPrefersExplicitFinal(t *testing.T) {
	info := &RelayInfo{
		RelayFormat:             types.RelayFormatOpenAI,
		RequestConversionChain:  []types.RelayFormat{types.RelayFormatOpenAI, types.RelayFormatClaude},
		FinalRequestRelayFormat: types.RelayFormatOpenAIResponses,
	}

	require.Equal(t, types.RelayFormat(types.RelayFormatOpenAIResponses), info.GetFinalRequestRelayFormat())
}

func TestRelayInfoGetFinalRequestRelayFormatFallsBackToConversionChain(t *testing.T) {
	info := &RelayInfo{
		RelayFormat:            types.RelayFormatOpenAI,
		RequestConversionChain: []types.RelayFormat{types.RelayFormatOpenAI, types.RelayFormatClaude},
	}

	require.Equal(t, types.RelayFormat(types.RelayFormatClaude), info.GetFinalRequestRelayFormat())
}

func TestRelayInfoGetFinalRequestRelayFormatFallsBackToRelayFormat(t *testing.T) {
	info := &RelayInfo{
		RelayFormat: types.RelayFormatGemini,
	}

	require.Equal(t, types.RelayFormat(types.RelayFormatGemini), info.GetFinalRequestRelayFormat())
}

func TestRelayInfoGetFinalRequestRelayFormatNilReceiver(t *testing.T) {
	var info *RelayInfo
	require.Equal(t, types.RelayFormat(""), info.GetFinalRequestRelayFormat())
}

func TestTaskSubmitReqUnmarshalDashScopeNativeFormat(t *testing.T) {
	raw := []byte(`{
		"model": "happyhorse-1.1-t2v",
		"input": {
			"prompt": "一座由硬纸板和瓶盖搭建的微型城市"
		},
		"parameters": {
			"resolution": "720P",
			"ratio": "16:9",
			"duration": 5
		}
	}`)

	var req TaskSubmitReq
	require.NoError(t, common.Unmarshal(raw, &req))
	require.Equal(t, "happyhorse-1.1-t2v", req.Model)
	require.Equal(t, "一座由硬纸板和瓶盖搭建的微型城市", req.Prompt)
	require.Equal(t, "720P", req.Resolution)
	require.Equal(t, 5, req.Duration)
	require.Equal(t, "16:9", req.Metadata["ratio"])
	require.NotNil(t, req.Metadata["input"])
	require.NotNil(t, req.Metadata["parameters"])
}

func TestTaskSubmitReqUnmarshalHappyHorseVideoEditParameters(t *testing.T) {
	raw := []byte(`{
		"model": "happyhorse-1.0-video-edit",
		"input": {
			"prompt": "让视频中的马头人身角色穿上图片中的条纹毛衣",
			"media": [
				{"type": "video", "url": "https://example.com/input.mp4"},
				{"type": "reference_image", "url": "https://example.com/ref.webp"}
			]
		},
		"parameters": {
			"resolution": "1080P",
			"duration": 6
		}
	}`)

	var req TaskSubmitReq
	require.NoError(t, common.Unmarshal(raw, &req))
	require.Equal(t, "happyhorse-1.0-video-edit", req.Model)
	require.Equal(t, "1080P", req.Resolution)
	require.Equal(t, 6, req.Duration)
	require.Equal(t, "1080P", req.DashScopeParameterString("resolution"))
}

func TestTaskSubmitReqMergeDashScopeParametersFromMetadata(t *testing.T) {
	req := TaskSubmitReq{
		Metadata: map[string]interface{}{
			"parameters": map[string]interface{}{
				"resolution": "720P",
				"duration":   8,
				"ratio":      "16:9",
			},
		},
	}
	req.NormalizeTaskRequest()
	require.Equal(t, "720P", req.Resolution)
	require.Equal(t, 8, req.Duration)
	require.Equal(t, "16:9", req.Metadata["ratio"])
}
