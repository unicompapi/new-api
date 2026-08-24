package service

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskInputMediaPersistsServesAndRemovesCapabilityURL(t *testing.T) {
	withTaskInputMediaSettings(t)
	header := multipartInputFile(t, pngBytes(), "image.png")
	relative, publicURL, err := PersistTaskInputImage(header)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(publicURL, "https://gateway.example/v1/video-inputs/"))

	token := filepath.Base(relative)
	path, mimeType, err := ResolveTaskInputMedia(token)
	require.NoError(t, err)
	assert.Equal(t, "image/png", mimeType)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, pngBytes(), data)

	RemoveTaskInputMedia(relative)
	_, _, err = ResolveTaskInputMedia(token)
	require.ErrorContains(t, err, "unavailable")
}

func TestTaskInputMediaRejectsEmptyOversizedAndUnsupportedFiles(t *testing.T) {
	withTaskInputMediaSettings(t)

	_, _, err := PersistTaskInputImage(multipartInputFile(t, nil, "empty.png"))
	require.ErrorContains(t, err, "must not be empty")

	wrong := multipartInputFile(t, []byte("plain text"), "not-image.txt")
	_, _, err = PersistTaskInputImage(wrong)
	require.ErrorContains(t, err, "unsupported MIME type")

	large := multipartInputFile(t, pngBytes(), "large.png")
	large.Size = maxTaskInputImageBytes + 1
	_, _, err = PersistTaskInputImage(large)
	require.ErrorContains(t, err, "20 MB limit")
}

func TestTaskInputMediaRequiresPublicHTTPSAddressAndExpires(t *testing.T) {
	withTaskInputMediaSettings(t)
	header := multipartInputFile(t, pngBytes(), "image.png")

	system_setting.ServerAddress = "http://127.0.0.1:3000"
	_, _, err := PersistTaskInputImage(header)
	require.ErrorContains(t, err, "public HTTPS")

	system_setting.ServerAddress = "https://gateway.example"
	relative, _, err := PersistTaskInputImage(header)
	require.NoError(t, err)
	path, err := ResolveTaskMediaFile(relative)
	require.NoError(t, err)
	old := time.Now().Add(-taskInputMediaTTL - time.Minute)
	require.NoError(t, os.Chtimes(path, old, old))
	CleanupExpiredTaskInputMedia()
	_, statErr := os.Stat(path)
	assert.ErrorIs(t, statErr, os.ErrNotExist)
}

func TestTaskInputMediaRejectsInvalidTokensAndTraversal(t *testing.T) {
	withTaskInputMediaSettings(t)
	_, _, err := ResolveTaskInputMedia("../secret")
	require.ErrorContains(t, err, "invalid input media token")

	outside := filepath.Join(filepath.Dir(constant.TaskMediaDir), "outside")
	require.NoError(t, os.WriteFile(outside, []byte("keep"), 0o600))
	RemoveTaskInputMedia(filepath.Join("..", "outside"))
	_, err = os.Stat(outside)
	require.NoError(t, err)
}

func withTaskInputMediaSettings(t *testing.T) {
	t.Helper()
	oldDir := constant.TaskMediaDir
	oldAddress := system_setting.ServerAddress
	constant.TaskMediaDir = t.TempDir()
	system_setting.ServerAddress = "https://gateway.example"
	t.Cleanup(func() {
		constant.TaskMediaDir = oldDir
		system_setting.ServerAddress = oldAddress
	})
}

func multipartInputFile(t *testing.T, data []byte, filename string) *multipart.FileHeader {
	t.Helper()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	part, err := w.CreateFormFile("input_reference", filename)
	require.NoError(t, err)
	_, err = part.Write(data)
	require.NoError(t, err)
	require.NoError(t, w.Close())
	req := httptest.NewRequest(http.MethodPost, "/v1/videos", &body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	require.NoError(t, req.ParseMultipartForm(1<<20))
	t.Cleanup(func() { _ = req.MultipartForm.RemoveAll() })
	return req.MultipartForm.File["input_reference"][0]
}

func pngBytes() []byte {
	return []byte("\x89PNG\r\n\x1a\nfixture")
}
