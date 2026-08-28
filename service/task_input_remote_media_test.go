package service

import (
	"bytes"
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPersistTaskInputRemoteMediaHTTPSAndRedirects(t *testing.T) {
	var base string
	base = withTaskInputRemoteFixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/png":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(pngBytes())
		case "/redirect":
			http.Redirect(w, r, base+"/png?fresh=signature", http.StatusFound)
		default:
			http.NotFound(w, r)
		}
	}))

	for _, path := range []string{"/png?X-Signature=secret", "/redirect"} {
		relative, publicURL, audit, err := PersistTaskInputRemoteMedia(context.Background(), base+path, "reference_image", time.Second)
		require.NoError(t, err)
		t.Cleanup(func() { RemoveTaskInputMedia(relative) })
		assert.Equal(t, "stabilized", audit.Stage)
		assert.Equal(t, "image/png", audit.ContentType)
		assert.NotZero(t, audit.Bytes)
		assert.NotContains(t, audit.Source, "Signature")
		assert.NotEmpty(t, audit.SourceHash)
		assert.NotEmpty(t, audit.ExpiresAtUTC)
		assert.True(t, strings.HasPrefix(publicURL, "https://gateway.example/v1/video-inputs/"))
		stored, mimeType, err := ResolveTaskInputMedia(strings.TrimPrefix(relative, "inputs/"))
		require.NoError(t, err)
		assert.Equal(t, "image/png", mimeType)
		data, err := os.ReadFile(stored)
		require.NoError(t, err)
		assert.Equal(t, pngBytes(), data)
	}
}

func TestPersistTaskInputRemoteMediaPreservesSupportedRoleFormats(t *testing.T) {
	fixtures := map[string]struct {
		mime string
		data []byte
	}{
		"/image": {"image/jpeg", jpegFixture()},
		"/video": {"video/mp4", mp4Fixture("isom")},
		"/audio": {"audio/wav", wavFixture()},
	}
	base := withTaskInputRemoteFixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fixture, ok := fixtures[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", fixture.mime)
		_, _ = w.Write(fixture.data)
	}))
	for _, tc := range []struct{ path, role, mime string }{
		{"/image", "first_image", "image/jpeg"},
		{"/image", "last_image", "image/jpeg"},
		{"/video", "reference_video", "video/mp4"},
		{"/audio", "reference_audio", "audio/wav"},
	} {
		relative, _, audit, err := PersistTaskInputRemoteMedia(context.Background(), base+tc.path, tc.role, time.Second)
		require.NoError(t, err)
		RemoveTaskInputMedia(relative)
		assert.Equal(t, tc.mime, audit.ContentType)
	}
}

func TestPersistTaskInputRemoteMediaRejectsHTTPAndBodyFailures(t *testing.T) {
	base := withTaskInputRemoteFixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/403":
			http.Error(w, "forbidden", http.StatusForbidden)
		case "/404":
			http.NotFound(w, r)
		case "/500":
			http.Error(w, "failed", http.StatusInternalServerError)
		case "/empty":
			w.WriteHeader(http.StatusOK)
		case "/mismatch":
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write(pngBytes())
		case "/wrong-role":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(pngBytes())
		case "/oversized-header":
			w.Header().Set("Content-Length", strconv.Itoa(maxTaskInputMediaBytes+1))
			w.WriteHeader(http.StatusOK)
		case "/oversized-stream":
			w.Header().Set("Content-Type", "video/mp4")
			w.Header().Set("Transfer-Encoding", "chunked")
			_, _ = w.Write(append(mp4Fixture("isom"), bytes.Repeat([]byte{'x'}, maxTaskInputMediaBytes)...))
		case "/interrupted":
			w.Header().Set("Content-Type", "image/png")
			w.Header().Set("Content-Length", "1000")
			_, _ = w.Write(pngBytes())
		default:
			http.NotFound(w, r)
		}
	}))

	for _, status := range []int{403, 404, 500} {
		_, _, audit, err := PersistTaskInputRemoteMedia(context.Background(), base+"/"+strconv.Itoa(status), "reference_image", time.Second)
		var remoteErr *TaskInputRemoteError
		require.ErrorAs(t, err, &remoteErr)
		assert.Equal(t, "http_status", audit.Stage)
		assert.Equal(t, status, remoteErr.HTTPStatus)
		assert.Contains(t, err.Error(), strconv.Itoa(status))
	}

	for _, tc := range []struct{ path, role, kind string }{
		{"/empty", "reference_image", "empty"},
		{"/mismatch", "reference_image", "mime"},
		{"/wrong-role", "reference_video", "mime"},
		{"/oversized-header", "reference_image", "too_large"},
		{"/oversized-stream", "reference_video", "too_large"},
		{"/interrupted", "reference_image", "interrupted"},
	} {
		_, _, _, err := PersistTaskInputRemoteMedia(context.Background(), base+tc.path, tc.role, 3*time.Second)
		var remoteErr *TaskInputRemoteError
		require.ErrorAs(t, err, &remoteErr)
		assert.Equal(t, tc.kind, remoteErr.Kind)
	}
}

func TestPersistTaskInputRemoteMediaRejectsTimeoutAndRedirectLoop(t *testing.T) {
	var base string
	base = withTaskInputRemoteFixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/slow":
			time.Sleep(80 * time.Millisecond)
			_, _ = w.Write(pngBytes())
		case "/loop":
			http.Redirect(w, r, base+"/loop", http.StatusFound)
		}
	}))
	_, _, _, err := PersistTaskInputRemoteMedia(context.Background(), base+"/slow", "reference_image", 10*time.Millisecond)
	var remoteErr *TaskInputRemoteError
	require.ErrorAs(t, err, &remoteErr)
	assert.Equal(t, "timeout", remoteErr.Kind)

	_, _, _, err = PersistTaskInputRemoteMedia(context.Background(), base+"/loop", "reference_image", time.Second)
	require.ErrorAs(t, err, &remoteErr)
	assert.Equal(t, "redirect", remoteErr.Kind)
}

func TestPersistTaskInputRemoteMediaBlocksSSRFAndDNSRebinding(t *testing.T) {
	withTaskInputRemoteSettings(t)
	for _, raw := range []string{
		"http://127.0.0.1/x", "http://10.0.0.1/x", "http://169.254.169.254/x",
		"http://[::1]/x", "http://[fc00::1]/x", "http://[fe80::1]/x",
	} {
		_, _, _, err := PersistTaskInputRemoteMedia(context.Background(), raw, "reference_image", time.Second)
		var remoteErr *TaskInputRemoteError
		require.ErrorAs(t, err, &remoteErr)
		assert.Equal(t, "unsafe_url", remoteErr.Kind)
	}

	taskInputLookupIPAddr = func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("192.168.1.10")}}, nil
	}
	_, _, _, err := PersistTaskInputRemoteMedia(context.Background(), "https://private.example/x", "reference_image", time.Second)
	var remoteErr *TaskInputRemoteError
	require.ErrorAs(t, err, &remoteErr)
	assert.Equal(t, "unsafe_url", remoteErr.Kind)

	taskInputLookupIPAddr = func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{
			{IP: net.ParseIP("93.184.216.34")},
			{IP: net.ParseIP("10.0.0.9")},
		}, nil
	}
	_, _, _, err = PersistTaskInputRemoteMedia(context.Background(), "https://mixed.example/x", "reference_image", time.Second)
	require.ErrorAs(t, err, &remoteErr)
	assert.Equal(t, "unsafe_url", remoteErr.Kind)

	var lookups atomic.Int32
	taskInputLookupIPAddr = func(context.Context, string) ([]net.IPAddr, error) {
		if lookups.Add(1) == 1 {
			return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}, nil
		}
		return []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}, nil
	}
	_, _, _, err = PersistTaskInputRemoteMedia(context.Background(), "https://rebind.example/x", "reference_image", time.Second)
	require.ErrorAs(t, err, &remoteErr)
	assert.Equal(t, "unsafe_url", remoteErr.Kind)
}

func TestPersistTaskInputRemoteMediaChecksEveryRedirect(t *testing.T) {
	var base string
	base = withTaskInputRemoteFixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/private" {
			http.Redirect(w, r, "http://127.0.0.1/secret", http.StatusFound)
			return
		}
		if r.URL.Path == "/public" {
			u, _ := url.Parse(base)
			http.Redirect(w, r, "https://redirect.example:"+u.Port()+"/png", http.StatusFound)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(pngBytes())
	}))
	_, _, _, err := PersistTaskInputRemoteMedia(context.Background(), base+"/private", "reference_image", time.Second)
	var remoteErr *TaskInputRemoteError
	require.ErrorAs(t, err, &remoteErr)
	assert.Equal(t, "redirect", remoteErr.Kind)

	relative, _, audit, err := PersistTaskInputRemoteMedia(context.Background(), base+"/public", "reference_image", time.Second)
	require.NoError(t, err)
	RemoveTaskInputMedia(relative)
	assert.Contains(t, audit.FinalSource, "redirect.example")
}

func withTaskInputRemoteFixture(t *testing.T, handler http.Handler) string {
	t.Helper()
	withTaskInputRemoteSettings(t)
	server := httptest.NewTLSServer(handler)
	t.Cleanup(server.Close)
	serverAddress := server.Listener.Addr().String()
	_, port, err := net.SplitHostPort(serverAddress)
	require.NoError(t, err)
	taskInputLookupIPAddr = func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}, nil
	}
	taskInputDialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, serverAddress)
	}
	taskInputTLSConfig = &tls.Config{InsecureSkipVerify: true} // fixture certificate only
	return "https://media.example:" + port
}

func withTaskInputRemoteSettings(t *testing.T) {
	t.Helper()
	oldDir, oldAddress := constant.TaskMediaDir, system_setting.ServerAddress
	oldLookup, oldDial, oldTLS := taskInputLookupIPAddr, taskInputDialContext, taskInputTLSConfig
	constant.TaskMediaDir = t.TempDir()
	system_setting.ServerAddress = "https://gateway.example"
	t.Cleanup(func() {
		constant.TaskMediaDir, system_setting.ServerAddress = oldDir, oldAddress
		taskInputLookupIPAddr, taskInputDialContext, taskInputTLSConfig = oldLookup, oldDial, oldTLS
	})
}

func jpegFixture() []byte {
	return []byte{0xff, 0xd8, 0xff, 0xe0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0x00, 0x01, 0xff, 0xd9}
}

func mp4Fixture(brand string) []byte {
	return append([]byte{0, 0, 0, 24, 'f', 't', 'y', 'p'}, append([]byte(brand), []byte{0, 0, 0, 0, 'i', 's', 'o', 'm'}...)...)
}

func wavFixture() []byte {
	return []byte("RIFF\x24\x00\x00\x00WAVEfmt \x10\x00\x00\x00\x01\x00\x01\x00\x40\x1f\x00\x00\x80\x3e\x00\x00\x02\x00\x10\x00data\x00\x00\x00\x00")
}
