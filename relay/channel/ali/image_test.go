package ali

import (
	"encoding/json"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/stretchr/testify/require"
)

func TestQwenImageModelsUseSynchronousDashScopeEndpoint(t *testing.T) {
	for _, model := range []string{
		"qwen-image-3.0",
		"qwen-image-3.0-pro",
		"qwen-image-2.0",
		"qwen-image-2.0-pro",
	} {
		t.Run(model, func(t *testing.T) {
			info := &relaycommon.RelayInfo{
				OriginModelName: model,
				RelayMode:       constant.RelayModeImagesGenerations,
				ChannelMeta: &relaycommon.ChannelMeta{
					ChannelBaseUrl: "https://dashscope.aliyuncs.com",
				},
			}
			url, err := (&Adaptor{}).GetRequestURL(info)
			require.NoError(t, err)
			require.Equal(t, "https://dashscope.aliyuncs.com/api/v1/services/aigc/multimodal-generation/generation", url)
			require.True(t, isSyncImageModel(model))
		})
	}
}

func TestQwenImageDashScopeRequestMapping(t *testing.T) {
	seed := json.RawMessage("123")
	promptExtend := json.RawMessage("false")
	enableThinking := json.RawMessage("true")
	image := json.RawMessage(`"https://example.com/input.png"`)
	require.NotEmpty(t, seed)

	for _, model := range []string{
		"qwen-image-3.0",
		"qwen-image-3.0-pro",
		"qwen-image-2.0",
		"qwen-image-2.0-pro",
	} {
		t.Run(model, func(t *testing.T) {
			n := uint(2)
			req := dto.ImageRequest{
				Model:  model,
				Prompt: "a red horse",
				N:      &n,
				Size:   "1024x1024",
				Image:  image,
				Extra: map[string]json.RawMessage{
					"negative_prompt":    json.RawMessage(`"blurry"`),
					"seed":               seed,
					"prompt_extend":      promptExtend,
					"prompt_extend_mode": json.RawMessage(`"direct"`),
					"enable_thinking":    enableThinking,
				},
			}
			aliReq, err := oaiImage2AliImageRequest(&relaycommon.RelayInfo{}, req, true)
			require.NoError(t, err)
			require.Equal(t, model, aliReq.Model)
			require.Equal(t, "1024*1024", aliReq.Parameters.Size)
			require.Equal(t, 2, aliReq.Parameters.N)
			require.Equal(t, "blurry", aliReq.Parameters.NegativePrompt)
			require.Equal(t, "direct", aliReq.Parameters.PromptExtendMode)
			require.NotNil(t, aliReq.Parameters.PromptExtend)
			require.False(t, *aliReq.Parameters.PromptExtend)
			require.NotNil(t, aliReq.Parameters.EnableThinking)
			require.True(t, *aliReq.Parameters.EnableThinking)
			require.NotNil(t, aliReq.Parameters.Seed)
			require.Equal(t, 123, *aliReq.Parameters.Seed)

			input, ok := aliReq.Input.(AliImageInput)
			require.True(t, ok)
			require.Len(t, input.Messages, 1)
			require.Equal(t, "user", input.Messages[0].Role)
			content, ok := input.Messages[0].Content.([]AliMediaContent)
			require.True(t, ok)
			require.Len(t, content, 2)
			require.Equal(t, "https://example.com/input.png", content[0].Image)
			require.Equal(t, "a red horse", content[1].Text)
		})
	}
}

func TestQwenImageRequestRejectsMoreThanThreeInputImages(t *testing.T) {
	req := dto.ImageRequest{
		Model:  "qwen-image-3.0",
		Prompt: "a red horse",
		Image:  json.RawMessage(`["https://example.com/1.png","https://example.com/2.png","https://example.com/3.png","https://example.com/4.png"]`),
	}
	_, err := oaiImage2AliImageRequest(&relaycommon.RelayInfo{}, req, true)
	require.ErrorContains(t, err, "at most 3 input images")
}

func TestQwenImageDefaultPrices(t *testing.T) {
	want := map[string]float64{
		"qwen-image-3.0":     0.18 / ratio_setting.USD2RMB,
		"qwen-image-3.0-pro": 0.25 / ratio_setting.USD2RMB,
		"qwen-image-2.0":     0.2 / ratio_setting.USD2RMB,
		"qwen-image-2.0-pro": 0.5 / ratio_setting.USD2RMB,
	}
	for model, expected := range want {
		price, ok := ratio_setting.GetDefaultModelPriceMap()[model]
		require.True(t, ok)
		require.InDelta(t, expected, price, 1e-9, model)
	}

	data, err := common.Marshal(want)
	require.NoError(t, err)
	require.NotEmpty(t, data)
}

func TestQwenImageProOutputPriceTier(t *testing.T) {
	require.Equal(t, float64(1), qwenImageOutputRatio("qwen-image-3.0-pro", "1024x2048"))
	require.Equal(t, float64(2), qwenImageOutputRatio("qwen-image-3.0-pro", "2048*2048"))
	require.Equal(t, float64(1), qwenImageOutputRatio("qwen-image-3.0", "2048*2048"))
}
