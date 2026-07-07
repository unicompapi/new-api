package ali

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/stretchr/testify/require"
)

const happyHorse11T2V1080PRequest = `{
	"model": "happyhorse-1.1-t2v",
	"input": {
		"prompt": "一座由硬纸板和瓶盖搭建的微型城市，在夜晚焕发出生机。一列硬纸板火车缓缓驶过，小灯点缀其间，照亮前路。"
	},
	"parameters": {
		"resolution": "1080P",
		"ratio": "16:9",
		"duration": 5
	}
}`

func TestHappyHorse11T2V1080PRequestParsing(t *testing.T) {
	var taskReq relaycommon.TaskSubmitReq
	require.NoError(t, common.Unmarshal([]byte(happyHorse11T2V1080PRequest), &taskReq))
	require.Equal(t, "happyhorse-1.1-t2v", taskReq.Model)
	require.Equal(t, "1080P", taskReq.Resolution)
	require.Equal(t, 5, taskReq.Duration)
	require.Equal(t, "16:9", taskReq.Metadata["ratio"])
}

func TestHappyHorse11T2V1080PBillingRatios(t *testing.T) {
	var taskReq relaycommon.TaskSubmitReq
	require.NoError(t, common.Unmarshal([]byte(happyHorse11T2V1080PRequest), &taskReq))

	info := &relaycommon.RelayInfo{OriginModelName: taskReq.Model}
	info.ChannelMeta = &relaycommon.ChannelMeta{}
	adaptor := &TaskAdaptor{}
	aliReq, err := adaptor.convertToAliRequest(info, taskReq)
	require.NoError(t, err)
	require.Equal(t, "1080P", aliReq.Parameters.Resolution)
	require.Equal(t, "16:9", aliReq.Parameters.Ratio)
	require.Equal(t, 5, aliReq.Parameters.Duration)

	ratios, err := ProcessAliOtherRatios(aliReq, taskReq.Model)
	require.NoError(t, err)
	require.InDelta(t, 4.0/3.0, ratios["resolution-1080P"], 0.0001)
	require.InDelta(t, 5.0, float64(aliReq.Parameters.Duration), 0.0001)
}

func TestHappyHorse11T2V1080PQuotaCNY(t *testing.T) {
	operation_setting.USDExchangeRate = 7.3

	var taskReq relaycommon.TaskSubmitReq
	require.NoError(t, common.Unmarshal([]byte(happyHorse11T2V1080PRequest), &taskReq))

	info := &relaycommon.RelayInfo{OriginModelName: taskReq.Model}
	info.ChannelMeta = &relaycommon.ChannelMeta{}
	adaptor := &TaskAdaptor{}
	aliReq, err := adaptor.convertToAliRequest(info, taskReq)
	require.NoError(t, err)

	modelPrice, ok := ratio_setting.GetDefaultModelPriceMap()[taskReq.Model]
	require.True(t, ok)

	baseQuota := int(ratio_setting.ModelPriceToUSD(taskReq.Model, modelPrice) * common.QuotaPerUnit)
	quota := baseQuota

	otherRatios, err := ProcessAliOtherRatios(aliReq, taskReq.Model)
	require.NoError(t, err)
	otherRatios["seconds"] = float64(aliReq.Parameters.Duration)

	for _, ra := range otherRatios {
		if ra != 1.0 {
			quota = int(float64(quota) * ra)
		}
	}

	cny := float64(quota) / common.QuotaPerUnit * operation_setting.USDExchangeRate
	require.InDelta(t, 6.0, cny, 0.05, "1080P 1.1 series 5s should cost about ¥6.0")
}
