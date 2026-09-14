package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/system_setting"
)

var taskMediaIDPattern = regexp.MustCompile(`^task_[A-Za-z0-9]+$`)

func PersistTaskResultMedia(ctx context.Context, channel *model.Channel, task *model.Task, result *relaycommon.TaskInfo) error {
	if !result.PersistResult {
		return nil
	}
	if !taskMediaIDPattern.MatchString(task.TaskID) {
		return errors.New("invalid task ID for media persistence")
	}
	root, err := filepath.Abs(constant.TaskMediaDir)
	if err != nil {
		return err
	}
	taskDir := filepath.Join(root, task.TaskID)
	if err := ensurePathWithin(root, taskDir); err != nil {
		return err
	}
	if err := os.MkdirAll(taskDir, 0o750); err != nil {
		return fmt.Errorf("create task media directory: %w", err)
	}

	videoRelative := filepath.Join(task.TaskID, "video.mp4")
	if err := downloadTaskMedia(ctx, channel, result.Url, filepath.Join(root, videoRelative)); err != nil {
		return fmt.Errorf("persist video: %w", err)
	}
	task.PrivateData.ResultFile = videoRelative
	task.PrivateData.ResultURL = taskcommon.BuildProxyURL(task.TaskID)

	if result.LastFrameURL != "" {
		frameRelative := filepath.Join(task.TaskID, "last_frame.jpg")
		if err := downloadTaskMedia(ctx, channel, result.LastFrameURL, filepath.Join(root, frameRelative)); err != nil {
			return fmt.Errorf("persist last frame: %w", err)
		}
		task.PrivateData.LastFrameFile = frameRelative
		task.PrivateData.LastFrameURL = taskcommon.BuildLastFrameProxyURL(task.TaskID)
	}
	return nil
}

func downloadTaskMedia(ctx context.Context, channel *model.Channel, sourceURL, target string) error {
	if strings.TrimSpace(sourceURL) == "" {
		return errors.New("source URL is empty")
	}
	fetchSetting := system_setting.GetFetchSetting()
	if err := common.ValidateURLWithFetchSetting(sourceURL, fetchSetting.EnableSSRFProtection, fetchSetting.AllowPrivateIp, fetchSetting.DomainFilterMode, fetchSetting.IpFilterMode, fetchSetting.DomainList, fetchSetting.IpList, fetchSetting.AllowedPorts, fetchSetting.ApplyIPFilterForDomain); err != nil {
		return fmt.Errorf("source URL blocked: %w", err)
	}
	if info, err := os.Stat(target); err == nil && info.Mode().IsRegular() && info.Size() > 0 {
		return nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	if err != nil {
		return err
	}
	client, err := GetHttpClientWithProxy(channel.GetSetting().Proxy)
	if err != nil {
		return err
	}
	clientCopy := *client
	timeout := channel.GetOtherSettings().TokenPonyMediaDownloadTimeoutSeconds
	if timeout > 0 {
		clientCopy.Timeout = time.Duration(timeout) * time.Second
	}
	resp, err := clientCopy.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("upstream returned HTTP %d", resp.StatusCode)
	}

	limit := int64(constant.MaxFileDownloadMB) * 1024 * 1024
	if limit <= 0 {
		return errors.New("MAX_FILE_DOWNLOAD_MB must be positive")
	}
	if resp.ContentLength > limit {
		return fmt.Errorf("media exceeds %d MB limit", constant.MaxFileDownloadMB)
	}
	temp, err := os.CreateTemp(filepath.Dir(target), ".download-*")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	written, copyErr := io.Copy(temp, io.LimitReader(resp.Body, limit+1))
	closeErr := temp.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	if written > limit {
		return fmt.Errorf("media exceeds %d MB limit", constant.MaxFileDownloadMB)
	}
	if written == 0 {
		return errors.New("upstream returned an empty media file")
	}
	if err := os.Rename(tempName, target); err != nil {
		if info, statErr := os.Stat(target); statErr == nil && info.Size() > 0 {
			return nil
		}
		return err
	}
	return nil
}

func ResolveTaskMediaFile(relative string) (string, error) {
	if strings.TrimSpace(relative) == "" {
		return "", errors.New("media file is not persisted")
	}
	root, err := filepath.Abs(constant.TaskMediaDir)
	if err != nil {
		return "", err
	}
	target, err := filepath.Abs(filepath.Join(root, relative))
	if err != nil {
		return "", err
	}
	if err := ensurePathWithin(root, target); err != nil {
		return "", err
	}
	return target, nil
}

func ensurePathWithin(root, target string) error {
	relative, err := filepath.Rel(root, target)
	if err != nil {
		return err
	}
	if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return errors.New("media path escapes task media directory")
	}
	return nil
}
