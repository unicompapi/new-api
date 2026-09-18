package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type downloadCountResponse struct {
	Success bool `json:"success"`
	Data    struct {
		Count int64 `json:"count"`
	} `json:"data"`
}

func setupDownloadControllerTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	previousDB := model.DB
	previousUsingSQLite := common.UsingSQLite
	previousUsingMySQL := common.UsingMySQL
	previousUsingPostgreSQL := common.UsingPostgreSQL

	gin.SetMode(gin.TestMode)
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.DownloadStat{}))
	model.DB = db

	t.Cleanup(func() {
		sqlDB, dbErr := db.DB()
		if dbErr == nil {
			_ = sqlDB.Close()
		}
		model.DB = previousDB
		common.UsingSQLite = previousUsingSQLite
		common.UsingMySQL = previousUsingMySQL
		common.UsingPostgreSQL = previousUsingPostgreSQL
	})

	return db
}

func invokeDownloadHandler(method string, target string, handler gin.HandlerFunc) *httptest.ResponseRecorder {
	engine := gin.New()
	engine.Handle(method, target, handler)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(method, target, nil)
	engine.ServeHTTP(recorder, request)
	return recorder
}

func decodeDownloadCountResponse(t *testing.T, recorder *httptest.ResponseRecorder) downloadCountResponse {
	t.Helper()

	var response downloadCountResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	return response
}

func TestGetUniCompWindowsDownloadCountStartsAtZero(t *testing.T) {
	setupDownloadControllerTestDB(t)

	recorder := invokeDownloadHandler(http.MethodGet, "/api/download/unicomp/windows/count", GetUniCompWindowsDownloadCount)

	require.Equal(t, http.StatusOK, recorder.Code)
	response := decodeDownloadCountResponse(t, recorder)
	require.True(t, response.Success)
	require.Zero(t, response.Data.Count)
}

func TestDownloadWindowsPreservesExistingRedirectAndCount(t *testing.T) {
	setupDownloadControllerTestDB(t)

	recorder := invokeDownloadHandler(http.MethodPost, "/api/download/windows", DownloadWindows)

	require.Equal(t, http.StatusFound, recorder.Code)
	require.Equal(t, windowsDownloadURL, recorder.Header().Get("Location"))
	windowsCount, err := model.GetDownloadCount(windowsDownloadKey)
	require.NoError(t, err)
	require.EqualValues(t, 1, windowsCount)
	uniCompCount, err := model.GetDownloadCount(uniCompWindowsDownloadKey)
	require.NoError(t, err)
	require.Zero(t, uniCompCount)
}

func TestDownloadUniCompWindowsIncrementsIndependentCountAndRedirects(t *testing.T) {
	setupDownloadControllerTestDB(t)
	_, err := model.IncrementDownloadCount(windowsDownloadKey)
	require.NoError(t, err)

	recorder := invokeDownloadHandler(http.MethodPost, "/api/download/unicomp/windows", DownloadUniCompWindows)

	require.Equal(t, http.StatusFound, recorder.Code)
	require.Equal(t, uniCompWindowsDownloadURL, recorder.Header().Get("Location"))
	uniCompCount, err := model.GetDownloadCount(uniCompWindowsDownloadKey)
	require.NoError(t, err)
	require.EqualValues(t, 1, uniCompCount)
	windowsCount, err := model.GetDownloadCount(windowsDownloadKey)
	require.NoError(t, err)
	require.EqualValues(t, 1, windowsCount)
}

// The counter is telemetry: if it cannot be written the installer must still
// be reachable, otherwise a statistics outage would break every download.
func TestDownloadStillRedirectsWhenCounterFails(t *testing.T) {
	db := setupDownloadControllerTestDB(t)

	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	recorder := invokeDownloadHandler(http.MethodPost, "/api/download/unicomp/windows", DownloadUniCompWindows)

	require.Equal(t, http.StatusFound, recorder.Code)
	require.Equal(t, uniCompWindowsDownloadURL, recorder.Header().Get("Location"))
}

func TestGetDownloadCountReportsFailureWhenDatabaseUnavailable(t *testing.T) {
	db := setupDownloadControllerTestDB(t)

	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	recorder := invokeDownloadHandler(http.MethodGet, "/api/download/unicomp/windows/count", GetUniCompWindowsDownloadCount)

	require.Equal(t, http.StatusInternalServerError, recorder.Code)
	response := decodeDownloadCountResponse(t, recorder)
	require.False(t, response.Success)
}
