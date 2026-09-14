package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeMainlandPhone(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "local", input: "13800138000", want: "+8613800138000"},
		{name: "e164", input: "+8613800138000", want: "+8613800138000"},
		{name: "international prefix", input: "0086 138-0013-8000", want: "+8613800138000"},
		{name: "country code", input: "8613800138000", want: "+8613800138000"},
		{name: "invalid prefix", input: "12800138000", wantErr: true},
		{name: "too short", input: "1380013800", wantErr: true},
		{name: "letters", input: "1380013800a", wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := NormalizeMainlandPhone(test.input)
			if test.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, test.want, got)
		})
	}
}

func TestGenerateSMSCode(t *testing.T) {
	pattern := regexp.MustCompile(`^\d{6}$`)
	for range 100 {
		code, err := generateSMSCode()
		require.NoError(t, err)
		assert.True(t, pattern.MatchString(code))
	}
}

func TestSMSCodeMaterialIsPurposeBound(t *testing.T) {
	phone := "+8613800138000"
	loginDigest := smsCodeDigest(phone, SMSPurposeLogin, "123456")
	registerDigest := smsCodeDigest(phone, SMSPurposeRegister, "123456")
	loginKey, _ := smsCodeKeys(phone, SMSPurposeLogin)

	assert.NotEqual(t, loginDigest, registerDigest)
	assert.NotContains(t, loginKey, phone)
}

func TestVerifyAliyunGraphCaptcha(t *testing.T) {
	oldID, oldKey, oldURL := common.AliyunGraphCaptchaAppID, common.AliyunGraphCaptchaAppKey, graphCaptchaURL
	defer func() {
		common.AliyunGraphCaptchaAppID = oldID
		common.AliyunGraphCaptchaAppKey = oldKey
		graphCaptchaURL = oldURL
	}()

	common.AliyunGraphCaptchaAppID = "captcha-id"
	common.AliyunGraphCaptchaAppKey = "key"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "captcha-id", r.URL.Query().Get("captcha_id"))
		require.NoError(t, r.ParseForm())
		require.Equal(t, "7616e815ce5687603d894a268012ede6bd2e15804a6095a8e4136588560a4c4b", r.Form.Get("sign_token"))
		_, _ = w.Write([]byte(`{"status":"success","result":"success"}`))
	}))
	defer server.Close()
	graphCaptchaURL = server.URL

	err := VerifyAliyunGraphCaptcha(context.Background(), AliyunGraphCaptchaValidation{
		LotNumber:     "lot-number",
		CaptchaOutput: "output",
		PassToken:     "token",
		GenTime:       "1234567890",
	})
	require.NoError(t, err)
	require.ErrorIs(t, VerifyAliyunGraphCaptcha(context.Background(), AliyunGraphCaptchaValidation{}), ErrGraphCaptcha)
}
