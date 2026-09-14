package controller

import (
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

type SMSCodeRequest struct {
	Phone        string                               `json:"phone"`
	Purpose      string                               `json:"purpose"`
	GraphCaptcha service.AliyunGraphCaptchaValidation `json:"graph_captcha"`
}

type SMSLoginRequest struct {
	Phone string `json:"phone"`
	Code  string `json:"code"`
}

func SendSMSCode(c *gin.Context) {
	var request SMSCodeRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}

	phone, err := service.NormalizeMainlandPhone(request.Phone)
	if err != nil {
		common.ApiErrorI18n(c, i18n.MsgUserPhoneInvalid)
		return
	}
	request.Purpose = strings.TrimSpace(request.Purpose)

	switch request.Purpose {
	case service.SMSPurposeLogin:
		if !service.SMSLoginAvailable() {
			common.ApiErrorI18n(c, i18n.MsgUserSMSUnavailable)
			return
		}
	case service.SMSPurposeRegister:
		if !common.RegisterEnabled {
			common.ApiErrorI18n(c, i18n.MsgUserRegisterDisabled)
			return
		}
		if !common.PasswordRegisterEnabled {
			common.ApiErrorI18n(c, i18n.MsgUserPasswordRegisterDisabled)
			return
		}
		if !common.SMSRegistrationRequired || !service.SMSVerificationAvailable() {
			common.ApiErrorI18n(c, i18n.MsgUserSMSUnavailable)
			return
		}
	default:
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	if err = service.VerifyAliyunGraphCaptcha(c.Request.Context(), request.GraphCaptcha); err != nil {
		common.ApiErrorI18n(c, i18n.MsgUserHumanVerificationFailed)
		return
	}

	// Reserve the same phone/IP allowance before checking account existence so
	// repeated requests cannot use rate-limit differences to enumerate users.
	if err = service.ReserveSMSVerificationSend(phone, c.ClientIP()); err != nil {
		handleSMSSendError(c, err)
		return
	}

	switch request.Purpose {
	case service.SMSPurposeLogin:
		if _, err = model.GetEnabledUserByPhone(phone); err != nil {
			if errors.Is(err, model.ErrDatabase) {
				common.SysLog(fmt.Sprintf("SMS login user lookup failed: %v", err))
				common.ApiErrorI18n(c, i18n.MsgDatabaseError)
				return
			}
			// Do not disclose whether a phone number is registered or disabled.
			common.ApiSuccess(c, gin.H{"retry_after": 60})
			return
		}
	case service.SMSPurposeRegister:
		exists, lookupErr := model.CheckPhoneExistOrDeleted(phone)
		if lookupErr != nil {
			common.SysLog(fmt.Sprintf("SMS registration phone lookup failed: %v", lookupErr))
			common.ApiErrorI18n(c, i18n.MsgDatabaseError)
			return
		}
		if exists {
			// Keep the response indistinguishable from a sent code.
			common.ApiSuccess(c, gin.H{"retry_after": 60})
			return
		}
	}

	if err = service.SendSMSVerificationCode(phone, request.Purpose); err != nil {
		handleSMSSendError(c, err)
		return
	}

	common.ApiSuccess(c, gin.H{"retry_after": 60})
}

func handleSMSSendError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrSMSRateLimited):
		common.ApiErrorI18n(c, i18n.MsgUserSMSTooFrequent)
	case errors.Is(err, service.ErrSMSUnavailable):
		common.ApiErrorI18n(c, i18n.MsgUserSMSUnavailable)
	default:
		common.ApiErrorI18n(c, i18n.MsgUserSMSSendFailed)
	}
}

func SMSLogin(c *gin.Context) {
	if !service.SMSLoginAvailable() {
		common.ApiErrorI18n(c, i18n.MsgUserSMSUnavailable)
		return
	}

	var request SMSLoginRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	phone, err := service.NormalizeMainlandPhone(request.Phone)
	if err != nil {
		common.ApiErrorI18n(c, i18n.MsgUserPhoneInvalid)
		return
	}
	if err = service.ConsumeSMSVerificationCode(phone, service.SMSPurposeLogin, request.Code); err != nil {
		if errors.Is(err, service.ErrSMSUnavailable) {
			common.ApiErrorI18n(c, i18n.MsgUserSMSUnavailable)
		} else {
			common.ApiErrorI18n(c, i18n.MsgUserPhoneOrCodeError)
		}
		return
	}

	user, err := model.GetEnabledUserByPhone(phone)
	if err != nil {
		if errors.Is(err, model.ErrDatabase) {
			common.SysLog(fmt.Sprintf("SMS login user lookup failed: %v", err))
			common.ApiErrorI18n(c, i18n.MsgDatabaseError)
		} else {
			common.ApiErrorI18n(c, i18n.MsgUserPhoneOrCodeError)
		}
		return
	}

	finishPrimaryLogin(user, c)
}
