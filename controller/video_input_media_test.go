package controller

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVideoInputMediaServesCapabilityURLWithoutAuthentication(t *testing.T) {
	oldDir := constant.TaskMediaDir
	oldAddress := system_setting.ServerAddress
	constant.TaskMediaDir = t.TempDir()
	system_setting.ServerAddress = "https://gateway.example"
	t.Cleanup(func() {
		constant.TaskMediaDir = oldDir
		system_setting.ServerAddress = oldAddress
	})

	header := controllerMultipartFile(t, []byte("\x89PNG\r\n\x1a\nfixture"))
	relative, _, err := service.PersistTaskInputImage(header)
	require.NoError(t, err)
	t.Cleanup(func() { service.RemoveTaskInputMedia(relative) })

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/v1/video-inputs/:token", VideoInputMedia)
	request := httptest.NewRequest(http.MethodGet, "/v1/video-inputs/"+filepath.Base(relative), nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, "image/png", recorder.Header().Get("Content-Type"))
	assert.Equal(t, "private, no-store", recorder.Header().Get("Cache-Control"))
	assert.Equal(t, "nosniff", recorder.Header().Get("X-Content-Type-Options"))
	assert.Equal(t, []byte("\x89PNG\r\n\x1a\nfixture"), recorder.Body.Bytes())

	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/v1/video-inputs/not-a-token", nil))
	assert.Equal(t, http.StatusNotFound, recorder.Code)
}

func controllerMultipartFile(t *testing.T, data []byte) *multipart.FileHeader {
	t.Helper()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	part, err := w.CreateFormFile("input_reference", "reference.png")
	require.NoError(t, err)
	_, err = part.Write(data)
	require.NoError(t, err)
	require.NoError(t, w.Close())
	request := httptest.NewRequest(http.MethodPost, "/v1/videos", &body)
	request.Header.Set("Content-Type", w.FormDataContentType())
	require.NoError(t, request.ParseMultipartForm(1<<20))
	t.Cleanup(func() { _ = request.MultipartForm.RemoveAll() })
	return request.MultipartForm.File["input_reference"][0]
}
