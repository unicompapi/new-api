package vidu

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/types"
)

func TestViduEffectiveBaselineCNYPerSec(t *testing.T) {
	tests := []struct {
		model     string
		reference bool
		want      float64
	}{
		{"viduq3-pro", false, 0.3125},
		{"viduq3-turbo", false, 0.1875},
		{"viduq3-turbo", true, 0.15625},
		{"viduq3-mix", true, 0.75},
		{"viduq3", true, 0.1875},
	}
	for _, tt := range tests {
		got := viduEffectiveBaselineCNYPerSec(tt.model, tt.reference)
		if got != tt.want {
			t.Fatalf("%s reference=%v: got %.5f want %.5f", tt.model, tt.reference, got, tt.want)
		}
	}
}

func TestViduOfficialCreditRateReferenceTurbo720p(t *testing.T) {
	rate := viduOfficialCreditRate("viduq3-turbo", "720p", true)
	if rate.offPeak != 5 || rate.peak != 10 {
		t.Fatalf("unexpected turbo reference 720p rate: %+v", rate)
	}
}

func TestAdjustBillingOnCompleteUsesPollCredits(t *testing.T) {
	taskData, err := common.Marshal(taskResultResponse{Credits: 25, State: "success"})
	if err != nil {
		t.Fatal(err)
	}

	task := &model.Task{
		Data: taskData,
		Properties: model.Properties{
			OriginModelName: "viduq3-turbo",
		},
		PrivateData: model.TaskPrivateData{
			BillingContext: &model.TaskBillingContext{
				OriginModelName: "viduq3-turbo",
				ModelPrice:      0.1875,
				GroupRatio:      1,
			},
		},
	}

	adaptor := &TaskAdaptor{}
	got := adaptor.AdjustBillingOnComplete(task, &relaycommon.TaskInfo{})
	if got <= 0 {
		t.Fatalf("expected positive quota, got %d", got)
	}

	want := viduQuotaFromCredits("viduq3-turbo", 25, 1)
	if got != want {
		t.Fatalf("got quota %d want %d (¥%.5f)", got, want, 25*viduCreditUnitPriceCNY)
	}
}

func TestAdjustBillingOnCompleteSkipsWhenSubmitAlreadySettled(t *testing.T) {
	taskData, err := common.Marshal(map[string]any{
		"credits": 60,
		"state":   "success",
	})
	if err != nil {
		t.Fatal(err)
	}

	quota := viduQuotaFromCredits("viduq3-pro-fast", 60, 1)
	task := &model.Task{
		Quota: quota,
		Data:  taskData,
		Properties: model.Properties{
			OriginModelName: "viduq3-pro-fast",
		},
		PrivateData: model.TaskPrivateData{
			BillingContext: &model.TaskBillingContext{
				OriginModelName: "viduq3-pro-fast",
				ModelPrice:      0.1875,
				GroupRatio:      1,
				OtherRatios:     map[string]float64{"credits-actual": 10},
			},
		},
	}

	adaptor := &TaskAdaptor{}
	got := adaptor.AdjustBillingOnComplete(task, &relaycommon.TaskInfo{})
	if got != 0 {
		t.Fatalf("expected skip duplicate settlement, got %d", got)
	}
}

func TestEstimateBillingReferenceTurboBaselineFactor(t *testing.T) {
	modelName := "viduq3-turbo"
	modelPrice := 0.1875
	effectiveBase := viduEffectiveBaselineCNYPerSec(modelName, true)
	factor := effectiveBase / modelPrice
	if factor < 0.8332 || factor > 0.8334 {
		t.Fatalf("expected baseline factor ~0.8333, got %.4f", factor)
	}
}

func TestSettleSubmitQuotaMatchesCompleteQuota(t *testing.T) {
	operation_setting.USDExchangeRate = 7.3

	taskData, err := common.Marshal(responsePayload{
		TaskId:  "974091781861867520",
		Credits: 145,
	})
	if err != nil {
		t.Fatal(err)
	}

	info := &relaycommon.RelayInfo{
		OriginModelName: "viduq3-mix",
		PriceData: types.PriceData{
			ModelPrice: 0.75,
			GroupRatioInfo: types.GroupRatioInfo{
				GroupRatio: 1,
			},
		},
	}

	adaptor := &TaskAdaptor{}
	submitQuota, _, ok := adaptor.SettleSubmitQuota(info, taskData)
	if !ok {
		t.Fatal("expected submit settlement")
	}

	completeQuota := viduQuotaFromCredits("viduq3-mix", 145, 1)
	if submitQuota != completeQuota {
		t.Fatalf("submit quota %d != complete quota %d", submitQuota, completeQuota)
	}
	if submitQuota != 310359 {
		t.Fatalf("unexpected quota %d want 310359", submitQuota)
	}
}

func TestResolveViduBillingParamsMetadataDurationOverridesTopLevel(t *testing.T) {
	req := relaycommon.TaskSubmitReq{
		Duration: 5,
		Metadata: map[string]interface{}{
			"duration": 16,
		},
	}
	params, err := resolveViduBillingParams(&req)
	if err != nil {
		t.Fatal(err)
	}
	if params.Duration != 16 {
		t.Fatalf("duration: got %d want 16", params.Duration)
	}
}

func TestResolveViduBillingParamsTopLevelResolutionOverridesMetadata(t *testing.T) {
	req := relaycommon.TaskSubmitReq{
		Resolution: "1080p",
		Metadata: map[string]interface{}{
			"resolution": "720p",
		},
	}
	params, err := resolveViduBillingParams(&req)
	if err != nil {
		t.Fatal(err)
	}
	if params.Resolution != "1080p" {
		t.Fatalf("resolution: got %q want 1080p", params.Resolution)
	}
}

func TestValidateViduBillingParamsRejectsDurationOutOfRange(t *testing.T) {
	cases := []struct {
		name     string
		action   string
		duration int
	}{
		{"text2video below min", constant.TaskActionTextGenerate, 0},
		{"text2video above max", constant.TaskActionTextGenerate, 17},
		{"reference below min", constant.TaskActionReferenceGenerate, 2},
		{"reference above max", constant.TaskActionReferenceGenerate, 17},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			params := viduBillingParams{Duration: tc.duration, Resolution: "720p"}
			if err := validateViduBillingParams(params, tc.action, "viduq3-pro"); err == nil {
				t.Fatal("expected rejection")
			}
		})
	}
}

func TestValidateViduBillingParamsRejectsInvalidResolution(t *testing.T) {
	params := viduBillingParams{Duration: 5, Resolution: "540p"}
	if err := validateViduBillingParams(params, constant.TaskActionReferenceGenerate, "viduq3-mix"); err == nil {
		t.Fatal("expected rejection for 540p on viduq3-mix reference")
	}
}

func TestConvertToRequestPayloadUsesResolvedBillingParams(t *testing.T) {
	adaptor := &TaskAdaptor{}
	req := relaycommon.TaskSubmitReq{
		Model:    "viduq3-pro",
		Prompt:   "test",
		Duration: 5,
		Metadata: map[string]interface{}{
			"duration": 8,
		},
	}
	info := &relaycommon.RelayInfo{
		TaskRelayInfo: &relaycommon.TaskRelayInfo{Action: constant.TaskActionTextGenerate},
	}
	body, err := adaptor.convertToRequestPayload(&req, info)
	if err != nil {
		t.Fatal(err)
	}
	if body.Duration != 8 {
		t.Fatalf("payload duration: got %d want 8", body.Duration)
	}
}
