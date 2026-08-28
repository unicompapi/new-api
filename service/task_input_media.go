package service

import (
	"encoding/base64"
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
	maxTaskInputMediaBytes = 20 << 20
	taskInputMediaTTL      = 24 * time.Hour
)

var taskInputTokenPattern = regexp.MustCompile(`^[A-Za-z0-9]{40}$`)

type taskInputPersistError struct {
	kind string
	err  error
}

func (e *taskInputPersistError) Error() string { return e.err.Error() }
func (e *taskInputPersistError) Unwrap() error { return e.err }

func newTaskInputPersistError(kind string, err error) error {
	return &taskInputPersistError{kind: kind, err: err}
}

func PersistTaskInputImage(fileHeader *multipart.FileHeader) (string, string, error) {
	if fileHeader == nil || fileHeader.Size == 0 {
		return "", "", errors.New("input_reference must not be empty")
	}
	if fileHeader.Size < 0 || fileHeader.Size > maxTaskInputMediaBytes {
		return "", "", fmt.Errorf("input_reference exceeds the %d MB limit", maxTaskInputMediaBytes>>20)
	}

	source, err := fileHeader.Open()
	if err != nil {
		return "", "", fmt.Errorf("open input_reference: %w", err)
	}
	defer source.Close()
	relative, publicURL, _, _, err := persistTaskInputMedia(source, "", "inline_image", "input_reference")
	return relative, publicURL, err
}

func PersistTaskInputImageDataURI(value string) (string, string, error) {
	header, payload, ok := strings.Cut(value, ",")
	if !ok || !strings.HasPrefix(strings.ToLower(header), "data:image/") {
		return "", "", errors.New("image must be a data:image/...;base64 URI")
	}
	mediaType, encoding, ok := strings.Cut(strings.TrimPrefix(strings.ToLower(header), "data:"), ";")
	if !ok || encoding != "base64" || !isSupportedTaskInputInlineImageType(mediaType) {
		return "", "", errors.New("base64 image must use image/jpeg, image/png, or image/webp")
	}
	if payload == "" {
		return "", "", errors.New("base64 image must not be empty")
	}
	if len(payload) > base64.StdEncoding.EncodedLen(maxTaskInputMediaBytes) {
		return "", "", fmt.Errorf("base64 image exceeds the %d MB limit", maxTaskInputMediaBytes>>20)
	}
	decoder := base64.NewDecoder(base64.StdEncoding.Strict(), strings.NewReader(payload))
	relative, publicURL, _, _, err := persistTaskInputMedia(decoder, mediaType, "inline_image", "base64 image")
	return relative, publicURL, err
}

func persistTaskInputMedia(source io.Reader, declaredMediaType, mediaRole, label string) (string, string, string, int64, error) {
	publicBase, err := taskInputPublicBaseURL()
	if err != nil {
		return "", "", "", 0, err
	}

	root, err := filepath.Abs(constant.TaskMediaDir)
	if err != nil {
		return "", "", "", 0, err
	}
	inputDir := filepath.Join(root, "inputs")
	if err := ensurePathWithin(root, inputDir); err != nil {
		return "", "", "", 0, err
	}
	if err := os.MkdirAll(inputDir, 0o750); err != nil {
		return "", "", "", 0, fmt.Errorf("create input media directory: %w", err)
	}

	temp, err := os.CreateTemp(inputDir, ".upload-*")
	if err != nil {
		return "", "", "", 0, err
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	written, copyErr := io.Copy(temp, io.LimitReader(source, maxTaskInputMediaBytes+1))
	if copyErr != nil {
		_ = temp.Close()
		return "", "", "", 0, newTaskInputPersistError("interrupted", fmt.Errorf("%s download interrupted: %w", label, copyErr))
	}
	if written == 0 {
		_ = temp.Close()
		return "", "", "", 0, newTaskInputPersistError("empty", fmt.Errorf("%s must not be empty", label))
	}
	if written > maxTaskInputMediaBytes {
		_ = temp.Close()
		return "", "", "", 0, newTaskInputPersistError("too_large", fmt.Errorf("%s exceeds the %d MB limit", label, maxTaskInputMediaBytes>>20))
	}
	if _, err := temp.Seek(0, io.SeekStart); err != nil {
		_ = temp.Close()
		return "", "", "", 0, err
	}
	header := make([]byte, 512)
	n, err := temp.Read(header)
	if err != nil && !errors.Is(err, io.EOF) {
		_ = temp.Close()
		return "", "", "", 0, err
	}
	mimeType := detectTaskInputMediaType(header[:n])
	if !isSupportedTaskInputMediaType(mediaRole, mimeType) {
		_ = temp.Close()
		return "", "", mimeType, 0, newTaskInputPersistError("mime", fmt.Errorf("%s has unsupported MIME type %q for %s", label, mimeType, mediaRole))
	}
	if declaredMediaType != "" && declaredMediaType != "application/octet-stream" && !sameTaskInputMediaType(declaredMediaType, mimeType) {
		_ = temp.Close()
		return "", "", mimeType, 0, newTaskInputPersistError("mime", fmt.Errorf("%s MIME mismatch: declared %s but detected %s", label, declaredMediaType, mimeType))
	}
	if err := temp.Close(); err != nil {
		return "", "", "", 0, err
	}

	token, err := common.GenerateRandomCharsKey(40)
	if err != nil {
		return "", "", "", 0, err
	}
	relative := filepath.Join("inputs", token)
	target := filepath.Join(root, relative)
	if err := ensurePathWithin(root, target); err != nil {
		return "", "", "", 0, err
	}
	if err := os.Rename(tempName, target); err != nil {
		return "", "", "", 0, err
	}
	return relative, publicBase + "/v1/video-inputs/" + token, mimeType, written, nil
}

func isSupportedTaskInputImageType(value string) bool {
	return isSupportedTaskInputInlineImageType(value) ||
		value == "image/gif" || value == "image/bmp" || value == "image/tiff" || value == "image/heic"
}

func isSupportedTaskInputInlineImageType(value string) bool {
	return value == "image/jpeg" || value == "image/png" || value == "image/webp"
}

func isSupportedTaskInputMediaType(role, value string) bool {
	switch role {
	case "inline_image":
		return isSupportedTaskInputInlineImageType(value)
	case "reference_image", "first_image", "last_image":
		return isSupportedTaskInputImageType(value)
	case "reference_video":
		return value == "video/mp4" || value == "video/quicktime"
	case "reference_audio":
		return value == "audio/mpeg" || value == "audio/wav" || value == "audio/mp4"
	default:
		return false
	}
}

func detectTaskInputMediaType(header []byte) string {
	if len(header) >= 12 && string(header[4:8]) == "ftyp" {
		switch string(header[8:12]) {
		case "qt  ":
			return "video/quicktime"
		case "M4A ", "M4B ":
			return "audio/mp4"
		case "heic", "heix", "hevc", "hevx", "heim", "heis", "mif1", "msf1":
			return "image/heic"
		case "isom", "iso2", "iso3", "iso4", "iso5", "iso6", "avc1", "mp41", "mp42", "dash", "M4V ", "M4VH", "M4VP":
			return "video/mp4"
		default:
			return "application/octet-stream"
		}
	}
	if len(header) >= 4 && (string(header[:4]) == "II*\x00" || string(header[:4]) == "MM\x00*") {
		return "image/tiff"
	}
	value := strings.ToLower(strings.TrimSpace(strings.Split(http.DetectContentType(header), ";")[0]))
	switch value {
	case "audio/x-wav", "audio/wave":
		return "audio/wav"
	case "image/jpg":
		return "image/jpeg"
	default:
		return value
	}
}

func sameTaskInputMediaType(declared, detected string) bool {
	declared = strings.ToLower(strings.TrimSpace(strings.Split(declared, ";")[0]))
	if declared == "binary/octet-stream" {
		declared = "application/octet-stream"
	}
	if declared == "image/jpg" {
		declared = "image/jpeg"
	}
	if declared == "image/heif" {
		declared = "image/heic"
	}
	if declared == "audio/x-wav" || declared == "audio/wave" {
		declared = "audio/wav"
	}
	return declared == detected
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
	return path, detectTaskInputMediaType(header[:n]), nil
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

func RemoveTaskInputMediaFiles(single string, files []string) {
	RemoveTaskInputMedia(single)
	for _, relative := range files {
		if relative != single {
			RemoveTaskInputMedia(relative)
		}
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
