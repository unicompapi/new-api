package controller

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTokenPonyAssetRejectsUndocumentedAndUnconfirmedLivenessActions(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("undocumented action", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Params = gin.Params{{Key: "action", Value: "CancelAsset"}}
		RelayTokenPonyAsset(ctx)
		assert.Equal(t, http.StatusNotFound, recorder.Code)
		assert.Contains(t, recorder.Body.String(), "unsupported_asset_action")
	})

	t.Run("liveness requires explicit confirmation", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Params = gin.Params{{Key: "action", Value: "CreateVisualValidateSession"}}
		ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/assets/CreateVisualValidateSession", nil)
		common.SetContextKey(ctx, constant.ContextKeyChannelType, constant.ChannelTypeTokenPony)
		RelayTokenPonyAsset(ctx)
		assert.Equal(t, http.StatusConflict, recorder.Code)
		assert.Contains(t, recorder.Body.String(), "liveness_confirmation_required")
	})
}

func TestTokenPonyAssetForwardsOnlyDocumentedEnvelope(t *testing.T) {
	service.InitHttpClient()
	var gotPath, gotAuth, gotBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ResponseMetadata":{"RequestId":"r1"},"Result":{"Id":"asset-1","Status":"Processing"}}`))
	}))
	defer server.Close()

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "action", Value: "CreateAsset"}}
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/assets/CreateAsset", strings.NewReader(`{"model":"doubao-seedance-2-5-260628","group":"default","Name":"portrait"}`))
	ctx.Request.Header.Set("Content-Type", "application/json")
	common.SetContextKey(ctx, constant.ContextKeyChannelType, constant.ChannelTypeTokenPony)
	common.SetContextKey(ctx, constant.ContextKeyChannelBaseUrl, server.URL)
	common.SetContextKey(ctx, constant.ContextKeyChannelKey, "fixture-key")

	RelayTokenPonyAsset(ctx)
	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, "/v1/assets/CreateAsset", gotPath)
	assert.Equal(t, "Bearer fixture-key", gotAuth)
	assert.JSONEq(t, `{"Name":"portrait"}`, gotBody)
	assert.Contains(t, recorder.Body.String(), `"Result"`)
}

func TestTokenPonyAssetTreatsResponseMetadataErrorAsFailure(t *testing.T) {
	service.InitHttpClient()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"ResponseMetadata":{"Error":{"Code":"Forbidden","Message":"denied"}}}`))
	}))
	defer server.Close()

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "action", Value: "GetAsset"}}
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/assets/GetAsset", strings.NewReader(`{"Id":"asset-1"}`))
	ctx.Request.Header.Set("Content-Type", "application/json")
	common.SetContextKey(ctx, constant.ContextKeyChannelType, constant.ChannelTypeTokenPony)
	common.SetContextKey(ctx, constant.ContextKeyChannelBaseUrl, server.URL)
	common.SetContextKey(ctx, constant.ContextKeyChannelKey, "fixture-key")

	RelayTokenPonyAsset(ctx)
	assert.Equal(t, http.StatusBadRequest, recorder.Code)
}
