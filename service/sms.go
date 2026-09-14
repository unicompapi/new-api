package service

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	openapi "github.com/alibabacloud-go/darabonba-openapi/v2/utils"
	dysmsapi20170525 "github.com/alibabacloud-go/dysmsapi-20170525/v5/client"
)

const (
	SMSPurposeLogin    = "login"
	SMSPurposeRegister = "register"

	smsCodeTTL        = 5 * time.Minute
	smsSendCooldown   = 60 * time.Second
	smsPhoneHourlyMax = 5
	smsIPHourlyMax    = 20
	smsCodeMaxTries   = 5
)

var (
	ErrSMSUnavailable = errors.New("短信服务暂不可用")
	ErrSMSRateLimited = errors.New("发送过于频繁，请稍后再试")
	ErrSMSSendFailed  = errors.New("验证码发送失败，请稍后再试")
	ErrSMSInvalidCode = errors.New("验证码错误或已过期")
	ErrGraphCaptcha   = errors.New("人机验证失败")

	mainlandPhonePattern = regexp.MustCompile(`^1[3-9]\d{9}$`)
	smsCodePattern       = regexp.MustCompile(`^\d{6}$`)
	graphCaptchaClient   = &http.Client{Timeout: 5 * time.Second}
	graphCaptchaURL      = "https://captcha.alicaptcha.com/validate"
)

type AliyunGraphCaptchaValidation struct {
	LotNumber     string `json:"lot_number"`
	CaptchaOutput string `json:"captcha_output"`
	PassToken     string `json:"pass_token"`
	GenTime       string `json:"gen_time"`
}

type aliyunGraphCaptchaResponse struct {
	Status string `json:"status"`
	Result string `json:"result"`
}

const smsRateLimitScript = `
local cooldown = redis.call("SET", KEYS[1], "1", "EX", ARGV[1], "NX")
if not cooldown then
  return -1
end

local phone_count = redis.call("INCR", KEYS[2])
if phone_count == 1 then
  redis.call("EXPIRE", KEYS[2], ARGV[2])
end

local ip_count = redis.call("INCR", KEYS[3])
if ip_count == 1 then
  redis.call("EXPIRE", KEYS[3], ARGV[2])
end

if phone_count > tonumber(ARGV[3]) or ip_count > tonumber(ARGV[4]) then
  redis.call("DEL", KEYS[1])
  return -2
end

return 1
`

const smsConsumeCodeScript = `
local expected = redis.call("GET", KEYS[1])
if not expected then
  return 0
end

local attempts = redis.call("INCR", KEYS[2])
if attempts == 1 then
  local ttl = redis.call("TTL", KEYS[1])
  if ttl > 0 then
    redis.call("EXPIRE", KEYS[2], ttl)
  end
end

if attempts > tonumber(ARGV[2]) then
  redis.call("DEL", KEYS[1], KEYS[2])
  return -1
end

if expected == ARGV[1] then
  redis.call("DEL", KEYS[1], KEYS[2])
  return 1
end

return 0
`

func SMSConfigured() bool {
	return common.RedisEnabled && common.RDB != nil &&
		strings.TrimSpace(common.AliyunSMSAccessKeyID) != "" &&
		strings.TrimSpace(common.AliyunSMSAccessKeySecret) != "" &&
		strings.TrimSpace(common.AliyunSMSSignName) != "" &&
		strings.TrimSpace(common.AliyunSMSTemplateCode) != ""
}

func SMSRequestProtectionConfigured() bool {
	return strings.TrimSpace(common.AliyunGraphCaptchaAppID) != "" &&
		strings.TrimSpace(common.AliyunGraphCaptchaAppKey) != ""
}

func SMSVerificationAvailable() bool {
	return SMSConfigured() && SMSRequestProtectionConfigured()
}

func SMSLoginAvailable() bool {
	return common.SMSLoginEnabled && SMSVerificationAvailable()
}

func VerifyAliyunGraphCaptcha(ctx context.Context, validation AliyunGraphCaptchaValidation) error {
	if !SMSRequestProtectionConfigured() {
		return ErrGraphCaptcha
	}

	validation.LotNumber = strings.TrimSpace(validation.LotNumber)
	validation.CaptchaOutput = strings.TrimSpace(validation.CaptchaOutput)
	validation.PassToken = strings.TrimSpace(validation.PassToken)
	validation.GenTime = strings.TrimSpace(validation.GenTime)
	if validation.LotNumber == "" || validation.CaptchaOutput == "" ||
		validation.PassToken == "" || validation.GenTime == "" ||
		len(validation.LotNumber) > 128 || len(validation.CaptchaOutput) > 4096 ||
		len(validation.PassToken) > 2048 || len(validation.GenTime) > 64 {
		return ErrGraphCaptcha
	}

	form := url.Values{
		"lot_number":     {validation.LotNumber},
		"captcha_output": {validation.CaptchaOutput},
		"pass_token":     {validation.PassToken},
		"gen_time":       {validation.GenTime},
		"sign_token":     {graphCaptchaSignToken(validation.LotNumber)},
	}
	endpoint := graphCaptchaURL + "?captcha_id=" + url.QueryEscape(strings.TrimSpace(common.AliyunGraphCaptchaAppID))
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return ErrGraphCaptcha
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	response, err := graphCaptchaClient.Do(request)
	if err != nil {
		common.SysLog("Aliyun graph captcha verification request failed")
		return ErrGraphCaptcha
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		common.SysLog(fmt.Sprintf("Aliyun graph captcha verification returned HTTP %d", response.StatusCode))
		return ErrGraphCaptcha
	}

	var result aliyunGraphCaptchaResponse
	if err = common.DecodeJson(io.LimitReader(response.Body, 64<<10), &result); err != nil ||
		result.Status != "success" || result.Result != "success" {
		return ErrGraphCaptcha
	}
	return nil
}

func graphCaptchaSignToken(lotNumber string) string {
	signer := hmac.New(sha256.New, []byte(strings.TrimSpace(common.AliyunGraphCaptchaAppKey)))
	_, _ = signer.Write([]byte(lotNumber))
	return hex.EncodeToString(signer.Sum(nil))
}

func NormalizeMainlandPhone(raw string) (string, error) {
	phone := strings.TrimSpace(raw)
	phone = strings.NewReplacer(" ", "", "-", "", "(", "", ")", "").Replace(phone)

	switch {
	case strings.HasPrefix(phone, "+86"):
		phone = strings.TrimPrefix(phone, "+86")
	case strings.HasPrefix(phone, "0086"):
		phone = strings.TrimPrefix(phone, "0086")
	case strings.HasPrefix(phone, "86") && len(phone) == 13:
		phone = strings.TrimPrefix(phone, "86")
	}

	if !mainlandPhonePattern.MatchString(phone) {
		return "", errors.New("手机号格式错误")
	}
	return "+86" + phone, nil
}

func ReserveSMSVerificationSend(phone, clientIP string) error {
	if !SMSVerificationAvailable() {
		return ErrSMSUnavailable
	}
	return acquireSMSSendLimit(phone, clientIP)
}

func SendSMSVerificationCode(phone, purpose string) error {
	if !validSMSPurpose(purpose) || !SMSVerificationAvailable() {
		return ErrSMSUnavailable
	}

	code, err := generateSMSCode()
	if err != nil {
		return ErrSMSSendFailed
	}

	codeKey, attemptsKey := smsCodeKeys(phone, purpose)
	ctx := context.Background()
	pipe := common.RDB.TxPipeline()
	pipe.Set(ctx, codeKey, smsCodeDigest(phone, purpose, code), smsCodeTTL)
	pipe.Del(ctx, attemptsKey)
	if _, err = pipe.Exec(ctx); err != nil {
		return ErrSMSUnavailable
	}

	if err = sendAliyunSMS(phone, code); err != nil {
		_ = common.RDB.Del(ctx, codeKey, attemptsKey).Err()
		return err
	}
	return nil
}

func ConsumeSMSVerificationCode(phone, purpose, code string) error {
	if !validSMSPurpose(purpose) || !SMSConfigured() {
		return ErrSMSUnavailable
	}
	if !smsCodePattern.MatchString(strings.TrimSpace(code)) {
		return ErrSMSInvalidCode
	}

	codeKey, attemptsKey := smsCodeKeys(phone, purpose)
	result, err := common.RDB.Eval(
		context.Background(),
		smsConsumeCodeScript,
		[]string{codeKey, attemptsKey},
		smsCodeDigest(phone, purpose, strings.TrimSpace(code)),
		smsCodeMaxTries,
	).Int()
	if err != nil {
		return ErrSMSUnavailable
	}
	if result != 1 {
		return ErrSMSInvalidCode
	}
	return nil
}

func validSMSPurpose(purpose string) bool {
	return purpose == SMSPurposeLogin || purpose == SMSPurposeRegister
}

func acquireSMSSendLimit(phone, clientIP string) error {
	phoneHash := common.GenerateHMAC("sms-phone:" + phone)
	ipHash := common.GenerateHMAC("sms-ip:" + strings.TrimSpace(clientIP))
	keys := []string{
		"sms:cooldown:" + phoneHash,
		"sms:hour:phone:" + phoneHash,
		"sms:hour:ip:" + ipHash,
	}
	result, err := common.RDB.Eval(
		context.Background(),
		smsRateLimitScript,
		keys,
		int(smsSendCooldown.Seconds()),
		int(time.Hour.Seconds()),
		smsPhoneHourlyMax,
		smsIPHourlyMax,
	).Int()
	if err != nil {
		return ErrSMSUnavailable
	}
	if result != 1 {
		return ErrSMSRateLimited
	}
	return nil
}

func generateSMSCode() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(900000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", n.Int64()+100000), nil
}

func smsCodeKeys(phone, purpose string) (string, string) {
	subject := common.GenerateHMAC("sms-code:" + purpose + ":" + phone)
	return "sms:code:" + subject, "sms:attempts:" + subject
}

func smsCodeDigest(phone, purpose, code string) string {
	return common.GenerateHMAC("sms-code:" + purpose + ":" + phone + ":" + code)
}

func sendAliyunSMS(phone, code string) error {
	connectTimeout := 3000
	readTimeout := 5000
	endpoint := "dysmsapi.aliyuncs.com"
	region := "cn-hangzhou"
	config := &openapi.Config{
		AccessKeyId:     &common.AliyunSMSAccessKeyID,
		AccessKeySecret: &common.AliyunSMSAccessKeySecret,
		Endpoint:        &endpoint,
		RegionId:        &region,
		ConnectTimeout:  &connectTimeout,
		ReadTimeout:     &readTimeout,
	}
	client, err := dysmsapi20170525.NewClient(config)
	if err != nil {
		common.SysLog("Aliyun SMS client initialization failed")
		return ErrSMSSendFailed
	}

	templateParam, err := common.Marshal(map[string]string{"code": code})
	if err != nil {
		return ErrSMSSendFailed
	}
	request := new(dysmsapi20170525.SendSmsRequest).
		SetPhoneNumbers(strings.TrimPrefix(phone, "+86")).
		SetSignName(common.AliyunSMSSignName).
		SetTemplateCode(common.AliyunSMSTemplateCode).
		SetTemplateParam(string(templateParam))

	response, err := client.SendSms(request)
	if err != nil {
		common.SysLog("Aliyun SMS request failed before a provider response was returned")
		return ErrSMSSendFailed
	}
	if response == nil || response.Body == nil || response.Body.Code == nil || *response.Body.Code != "OK" {
		providerCode := "empty_response"
		requestID := ""
		if response != nil && response.Body != nil {
			if response.Body.Code != nil {
				providerCode = *response.Body.Code
			}
			if response.Body.RequestId != nil {
				requestID = *response.Body.RequestId
			}
		}
		common.SysLog(fmt.Sprintf("Aliyun SMS rejected request: code=%s request_id=%s", providerCode, requestID))
		return ErrSMSSendFailed
	}
	return nil
}
