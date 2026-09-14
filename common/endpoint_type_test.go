package common

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
)

func TestGetEndpointTypesByChannelTypeDetectsTokenPonyCapabilities(t *testing.T) {
	tests := []struct {
		name        string
		channelType int
		modelName   string
		want        constant.EndpointType
	}{
		{name: "video channel", channelType: constant.ChannelTypeTokenPony, modelName: "doubao-seedance-2-5-260628", want: constant.EndpointTypeOpenAIVideo},
		{name: "embedding name", channelType: constant.ChannelTypeOpenAI, modelName: "qwen3-embedding-4b", want: constant.EndpointTypeEmbeddings},
		{name: "bge embedding name", channelType: constant.ChannelTypeOpenAI, modelName: "bge-m3", want: constant.EndpointTypeEmbeddings},
		{name: "rerank name", channelType: constant.ChannelTypeOpenAI, modelName: "qwen3-reranker-8b", want: constant.EndpointTypeJinaRerank},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetEndpointTypesByChannelType(tt.channelType, tt.modelName)
			if len(got) != 1 || got[0] != tt.want {
				t.Fatalf("GetEndpointTypesByChannelType() = %v, want [%s]", got, tt.want)
			}
		})
	}
}

func TestOpenAIVideoDefaultEndpoint(t *testing.T) {
	got, ok := GetDefaultEndpointInfo(constant.EndpointTypeOpenAIVideo)
	if !ok {
		t.Fatal("OpenAI video endpoint default is missing")
	}
	if got.Path != "/v1/videos" || got.Method != "POST" {
		t.Fatalf("OpenAI video endpoint = %+v", got)
	}
}
