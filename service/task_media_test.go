package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPersistTaskResultMediaDownloadsVideoAndLastFrame(t *testing.T) {
	withTaskMediaTestSettings(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/video":
			_, _ = w.Write([]byte("video-bytes"))
		case "/frame":
			_, _ = w.Write([]byte("frame-bytes"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	task := &model.Task{TaskID: "task_abc123"}
	result := &relaycommon.TaskInfo{
		Url:           server.URL + "/video?signature=short-lived",
		LastFrameURL:  server.URL + "/frame?signature=short-lived",
		PersistResult: true,
	}
	require.NoError(t, PersistTaskResultMedia(context.Background(), &model.Channel{}, task, result))

	assert.Contains(t, task.PrivateData.ResultURL, "/v1/videos/task_abc123/content")
	assert.Contains(t, task.PrivateData.LastFrameURL, "/v1/videos/task_abc123/last_frame")
	videoPath, err := ResolveTaskMediaFile(task.PrivateData.ResultFile)
	require.NoError(t, err)
	framePath, err := ResolveTaskMediaFile(task.PrivateData.LastFrameFile)
	require.NoError(t, err)
	video, err := os.ReadFile(videoPath)
	require.NoError(t, err)
	frame, err := os.ReadFile(framePath)
	require.NoError(t, err)
	assert.Equal(t, "video-bytes", string(video))
	assert.Equal(t, "frame-bytes", string(frame))
}

func TestPersistTaskResultMediaFailsClosedOnDownloadError(t *testing.T) {
	withTaskMediaTestSettings(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "expired", http.StatusForbidden)
	}))
	defer server.Close()

	task := &model.Task{TaskID: "task_downloadfail"}
	err := PersistTaskResultMedia(context.Background(), &model.Channel{}, task, &relaycommon.TaskInfo{
		Url:           server.URL + "/expired",
		PersistResult: true,
	})
	require.ErrorContains(t, err, "upstream returned HTTP 403")
	assert.Empty(t, task.PrivateData.ResultFile)
}

func TestResolveTaskMediaFileRejectsTraversal(t *testing.T) {
	withTaskMediaTestSettings(t)
	_, err := ResolveTaskMediaFile(filepath.Join("..", "secret"))
	require.ErrorContains(t, err, "escapes task media directory")

	task := &model.Task{TaskID: "../escape"}
	err = PersistTaskResultMedia(context.Background(), &model.Channel{}, task, &relaycommon.TaskInfo{PersistResult: true})
	require.ErrorContains(t, err, "invalid task ID")
}

func withTaskMediaTestSettings(t *testing.T) {
	t.Helper()
	oldDir := constant.TaskMediaDir
	oldLimit := constant.MaxFileDownloadMB
	fetch := system_setting.GetFetchSetting()
	oldFetch := *fetch
	constant.TaskMediaDir = t.TempDir()
	constant.MaxFileDownloadMB = 1
	fetch.EnableSSRFProtection = false
	InitHttpClient()
	t.Cleanup(func() {
		constant.TaskMediaDir = oldDir
		constant.MaxFileDownloadMB = oldLimit
		*fetch = oldFetch
		InitHttpClient()
	})
}
