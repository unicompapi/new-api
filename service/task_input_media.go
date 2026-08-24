package service

import (
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/setting/system_setting"
)

const (
	maxTaskInputImageBytes = 20 << 20
	taskInputMediaTTL      = 24 * time.Hour
)

var taskInputTokenPattern = regexp.MustCompile(`^[A-Za-z0-9]{40}$`)

func PersistTaskInputImage(fileHeader *multipart.FileHeader) (string, string, error) {
	publicBase, err := taskInputPublicBaseURL()
	if err != nil {
		return "", "", err
	}
	if fileHeader == nil || fileHeader.Size == 0 {
		return "", "", errors.New("input_reference must not be empty")
	}
	if fileHeader.Size < 0 || fileHeader.Size > maxTaskInputImageBytes {
		return "", "", fmt.Errorf("input_reference exceeds the %d MB limit", maxTaskInputImageBytes>>20)
	}

	source, err := fileHeader.Open()
	if err != nil {
		return "", "", fmt.Errorf("open input_reference: %w", err)
	}
	defer source.Close()

	root, err := filepath.Abs(constant.TaskMediaDir)
	if err != nil {
		return "", "", err
	}
	inputDir := filepath.Join(root, "inputs")
	if err := ensurePathWithin(root, inputDir); err != nil {
		return "", "", err
	}
	if err := os.MkdirAll(inputDir, 0o750); err != nil {
		return "", "", fmt.Errorf("create input media directory: %w", err)
	}

	temp, err := os.CreateTemp(inputDir, ".upload-*")
	if err != nil {
		return "", "", err
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	written, copyErr := io.Copy(temp, io.LimitReader(source, maxTaskInputImageBytes+1))
	if copyErr != nil {
		_ = temp.Close()
		return "", "", copyErr
	}
	if written == 0 {
		_ = temp.Close()
		return "", "", errors.New("input_reference must not be empty")
	}
	if written > maxTaskInputImageBytes {
		_ = temp.Close()
		return "", "", fmt.Errorf("input_reference exceeds the %d MB limit", maxTaskInputImageBytes>>20)
	}
	if _, err := temp.Seek(0, io.SeekStart); err != nil {
		_ = temp.Close()
		return "", "", err
	}
	header := make([]byte, 512)
	n, err := temp.Read(header)
	if err != nil && !errors.Is(err, io.EOF) {
		_ = temp.Close()
		return "", "", err
	}
	mimeType := http.DetectContentType(header[:n])
	if mimeType != "image/jpeg" && mimeType != "image/png" && mimeType != "image/webp" {
		_ = temp.Close()
		return "", "", fmt.Errorf("input_reference has unsupported MIME type %q; expected image/jpeg, image/png, or image/webp", mimeType)
	}
	if err := temp.Close(); err != nil {
		return "", "", err
	}

	token, err := common.GenerateRandomCharsKey(40)
	if err != nil {
		return "", "", err
	}
	relative := filepath.Join("inputs", token)
	target := filepath.Join(root, relative)
	if err := ensurePathWithin(root, target); err != nil {
		return "", "", err
	}
	if err := os.Rename(tempName, target); err != nil {
		return "", "", err
	}
	return relative, publicBase + "/v1/video-inputs/" + token, nil
}

func ResolveTaskInputMedia(token string) (string, string, error) {
	if !taskInputTokenPattern.MatchString(token) {
		return "", "", errors.New("invalid input media token")
	}
	path, err := ResolveTaskMediaFile(filepath.Join("inputs", token))
	if err != nil {
		return "", "", err
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
		return "", "", errors.New("input media is unavailable")
	}
	if time.Since(info.ModTime()) > taskInputMediaTTL {
		_ = os.Remove(path)
		return "", "", errors.New("input media has expired")
	}
	file, err := os.Open(path)
	if err != nil {
		return "", "", err
	}
	defer file.Close()
	header := make([]byte, 512)
	n, err := file.Read(header)
	if err != nil && !errors.Is(err, io.EOF) {
		return "", "", err
	}
	return path, http.DetectContentType(header[:n]), nil
}

func RemoveTaskInputMedia(relative string) {
	if relative == "" {
		return
	}
	clean := filepath.Clean(relative)
	token := filepath.Base(clean)
	if clean != filepath.Join("inputs", token) || !taskInputTokenPattern.MatchString(token) {
		return
	}
	if path, err := ResolveTaskMediaFile(clean); err == nil {
		_ = os.Remove(path)
	}
}

func CleanupExpiredTaskInputMedia() {
	root, err := filepath.Abs(constant.TaskMediaDir)
	if err != nil {
		return
	}
	entries, err := os.ReadDir(filepath.Join(root, "inputs"))
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-taskInputMediaTTL)
	for _, entry := range entries {
		if entry.IsDir() || (!taskInputTokenPattern.MatchString(entry.Name()) && !strings.HasPrefix(entry.Name(), ".upload-")) {
			continue
		}
		info, err := entry.Info()
		if err == nil && info.ModTime().Before(cutoff) {
			_ = os.Remove(filepath.Join(root, "inputs", entry.Name()))
		}
	}
}

func taskInputPublicBaseURL() (string, error) {
	base := strings.TrimRight(strings.TrimSpace(system_setting.ServerAddress), "/")
	u, err := url.Parse(base)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", errors.New("SERVER_ADDRESS must be a public HTTPS URL before input_reference files can be relayed")
	}
	host := strings.ToLower(u.Hostname())
	if host == "localhost" || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".local") {
		return "", errors.New("SERVER_ADDRESS must be publicly reachable before input_reference files can be relayed")
	}
	if ip := net.ParseIP(host); ip != nil && (ip.IsPrivate() || ip.IsLoopback() || ip.IsUnspecified() || ip.IsLinkLocalUnicast()) {
		return "", errors.New("SERVER_ADDRESS must be publicly reachable before input_reference files can be relayed")
	}
	return base, nil
}
