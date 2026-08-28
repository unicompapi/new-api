package service

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"errors"
	"fmt"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
)

type TaskInputRemoteAudit struct {
	Source        string `json:"source"`
	SourceHash    string `json:"source_hash"`
	FinalSource   string `json:"final_source,omitempty"`
	FinalHash     string `json:"final_hash,omitempty"`
	Role          string `json:"role"`
	Stage         string `json:"stage"`
	ErrorCode     string `json:"error_code,omitempty"`
	ContentType   string `json:"content_type,omitempty"`
	DetectedType  string `json:"detected_type,omitempty"`
	ContentLength *int64 `json:"content_length,omitempty"`
	Bytes         int64  `json:"bytes"`
	ExpiresAtUTC  string `json:"expires_at_utc,omitempty"`
}

type TaskInputRemoteError struct {
	Kind          string
	Stage         string
	HTTPStatus    int
	Detected      string
	Bytes         int64
	ContentLength *int64
}

func (e *TaskInputRemoteError) Error() string {
	switch e.Kind {
	case "unsafe_url":
		return "URL 被安全策略拒绝"
	case "redirect":
		return "重定向被安全策略拒绝或次数过多"
	case "http_status":
		return fmt.Sprintf("下载失败（HTTP %d）", e.HTTPStatus)
	case "timeout":
		return formatTaskInputTransferError("下载超时", e.Bytes, e.ContentLength)
	case "interrupted":
		return formatTaskInputTransferError("下载中断", e.Bytes, e.ContentLength)
	case "empty":
		return "文件为空"
	case "too_large":
		return fmt.Sprintf("超过 %dMB", maxTaskInputMediaBytes>>20)
	case "mime":
		if e.Detected != "" {
			return fmt.Sprintf("媒体格式与角色不匹配（检测为 %s）", e.Detected)
		}
		return "媒体 MIME 或文件魔数不匹配"
	default:
		return "下载或落盘失败"
	}
}

var (
	taskInputLookupIPAddr = net.DefaultResolver.LookupIPAddr
	taskInputDialContext  = (&net.Dialer{}).DialContext
	taskInputTLSConfig    *tls.Config
	taskInputRemoteClient = newTaskInputRemoteClient
)

func PersistTaskInputRemoteMedia(ctx context.Context, sourceURL, mediaRole string, timeout time.Duration) (string, string, TaskInputRemoteAudit, error) {
	audit := newTaskInputRemoteAudit(sourceURL, mediaRole)
	if _, err := validateTaskInputRemoteURL(ctx, sourceURL); err != nil {
		audit.Stage = "validate_url"
		audit.ErrorCode = taskInputRemoteErrorCode("unsafe_url")
		return "", "", audit, &TaskInputRemoteError{Kind: "unsafe_url", Stage: audit.Stage}
	}

	client := taskInputRemoteClient(timeout)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	if err != nil {
		audit.Stage = "build_request"
		audit.ErrorCode = taskInputRemoteErrorCode("unsafe_url")
		return "", "", audit, &TaskInputRemoteError{Kind: "unsafe_url", Stage: audit.Stage}
	}
	resp, err := client.Do(req)
	if err != nil {
		audit.Stage = "download"
		kind := "download"
		var urlErr *url.Error
		if errors.As(err, &urlErr) && strings.Contains(strings.ToLower(urlErr.Err.Error()), "redirect") {
			kind, audit.Stage = "redirect", "redirect"
		} else if errors.Is(err, context.DeadlineExceeded) || isTaskInputTimeout(err) {
			kind = "timeout"
		} else if strings.Contains(strings.ToLower(err.Error()), "private") || strings.Contains(strings.ToLower(err.Error()), "blocked") {
			kind = "unsafe_url"
		}
		audit.Stage = kind
		audit.ErrorCode = taskInputRemoteErrorCode(kind)
		return "", "", audit, &TaskInputRemoteError{Kind: kind, Stage: audit.Stage}
	}
	defer resp.Body.Close()

	audit.FinalSource = safeTaskInputSource(resp.Request.URL.String())
	audit.FinalHash = taskInputSourceHash(resp.Request.URL.String())
	if resp.ContentLength >= 0 {
		contentLength := resp.ContentLength
		audit.ContentLength = &contentLength
	}
	if resp.StatusCode != http.StatusOK {
		audit.Stage = "http_status"
		audit.ErrorCode = taskInputRemoteErrorCode("http_status")
		return "", "", audit, &TaskInputRemoteError{Kind: "http_status", Stage: audit.Stage, HTTPStatus: resp.StatusCode}
	}
	if resp.ContentLength > maxTaskInputMediaBytes {
		audit.Stage = "size"
		audit.ErrorCode = taskInputRemoteErrorCode("too_large")
		return "", "", audit, &TaskInputRemoteError{Kind: "too_large", Stage: audit.Stage}
	}

	declaredType := ""
	if value := strings.TrimSpace(resp.Header.Get("Content-Type")); value != "" {
		declaredType, _, err = mime.ParseMediaType(value)
		if err != nil {
			audit.Stage = "mime"
			audit.ErrorCode = taskInputRemoteErrorCode("mime")
			return "", "", audit, &TaskInputRemoteError{Kind: "mime", Stage: audit.Stage}
		}
		declaredType = strings.ToLower(declaredType)
		if declaredType == "binary/octet-stream" {
			declaredType = "application/octet-stream"
		}
		audit.ContentType = declaredType
	}
	relative, publicURL, detected, written, err := persistTaskInputMedia(resp.Body, declaredType, mediaRole, "remote media")
	audit.Bytes = written
	audit.DetectedType = detected
	if err != nil {
		kind := "persist"
		var persistErr *taskInputPersistError
		if errors.As(err, &persistErr) {
			kind = persistErr.kind
		}
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) || isTaskInputTimeout(err) {
			kind = "timeout"
		}
		audit.Stage = kind
		audit.ErrorCode = taskInputRemoteErrorCode(kind)
		return "", "", audit, &TaskInputRemoteError{Kind: kind, Stage: audit.Stage, Detected: detected, Bytes: written, ContentLength: audit.ContentLength}
	}
	audit.Stage = "stabilized"
	audit.DetectedType = detected
	audit.Bytes = written
	audit.ExpiresAtUTC = time.Now().Add(taskInputMediaTTL).UTC().Format(time.RFC3339)
	return relative, publicURL, audit, nil
}

func formatTaskInputTransferError(label string, bytes int64, contentLength *int64) string {
	if bytes <= 0 {
		return label
	}
	if contentLength != nil {
		return fmt.Sprintf("%s（已读取 %d/%d 字节）", label, bytes, *contentLength)
	}
	return fmt.Sprintf("%s（已读取 %d 字节）", label, bytes)
}

func taskInputRemoteErrorCode(kind string) string {
	return "media_" + kind
}

func newTaskInputRemoteClient(timeout time.Duration) *http.Client {
	transport := &http.Transport{
		Proxy:             nil,
		ForceAttemptHTTP2: true,
		DialContext:       dialPublicTaskInputAddress,
	}
	if taskInputTLSConfig != nil {
		transport.TLSClientConfig = taskInputTLSConfig.Clone()
	}
	return &http.Client{
		Transport: transport,
		Timeout:   timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return errors.New("redirect limit exceeded")
			}
			if _, err := validateTaskInputRemoteURL(req.Context(), req.URL.String()); err != nil {
				return errors.New("redirect blocked")
			}
			return nil
		},
	}
}

func validateTaskInputRemoteURL(ctx context.Context, value string) (*url.URL, error) {
	u, err := url.Parse(value)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil || u.Fragment != "" {
		return nil, errors.New("invalid public HTTP(S) URL")
	}
	if port := u.Port(); port != "" {
		parsed, err := strconv.Atoi(port)
		if err != nil || parsed < 1 || parsed > 65535 {
			return nil, errors.New("invalid URL port")
		}
	}
	if _, err := publicTaskInputIPs(ctx, u.Hostname()); err != nil {
		return nil, err
	}
	return u, nil
}

func dialPublicTaskInputAddress(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, errors.New("invalid remote address")
	}
	ips, err := publicTaskInputIPs(ctx, host)
	if err != nil {
		return nil, err
	}
	var lastErr error
	for _, ip := range ips {
		conn, err := taskInputDialContext(ctx, network, net.JoinHostPort(ip.String(), port))
		if err == nil {
			return conn, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = errors.New("no public address available")
	}
	return nil, lastErr
}

func publicTaskInputIPs(ctx context.Context, host string) ([]net.IP, error) {
	protection := &common.SSRFProtection{DomainFilterMode: false, IpFilterMode: false}
	if strings.Contains(host, "%") {
		return nil, errors.New("scoped IP addresses are blocked")
	}
	if ip := net.ParseIP(host); ip != nil {
		if !protection.IsIPAccessAllowed(ip) {
			return nil, errors.New("private or special IP address blocked")
		}
		return []net.IP{ip}, nil
	}
	addresses, err := taskInputLookupIPAddr(ctx, host)
	if err != nil || len(addresses) == 0 {
		return nil, errors.New("DNS resolution failed")
	}
	ips := make([]net.IP, 0, len(addresses))
	for _, address := range addresses {
		if !protection.IsIPAccessAllowed(address.IP) {
			return nil, errors.New("private or special DNS result blocked")
		}
		ips = append(ips, address.IP)
	}
	return ips, nil
}

func newTaskInputRemoteAudit(sourceURL, role string) TaskInputRemoteAudit {
	return TaskInputRemoteAudit{
		Source:     safeTaskInputSource(sourceURL),
		SourceHash: taskInputSourceHash(sourceURL),
		Role:       role,
		Stage:      "received",
	}
}

func safeTaskInputSource(value string) string {
	u, err := url.Parse(value)
	if err != nil || u.Host == "" {
		return "invalid-url"
	}
	path := u.EscapedPath()
	if path == "" {
		path = "/"
	}
	return strings.ToLower(u.Scheme) + "://" + u.Host + path
}

func taskInputSourceHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return fmt.Sprintf("sha256:%x", sum[:12])
}

func isTaskInputTimeout(err error) bool {
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}
