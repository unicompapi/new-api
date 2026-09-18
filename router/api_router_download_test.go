package router

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSetApiRouterRegistersUniCompWindowsDownloadRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	SetApiRouter(engine)

	routes := make(map[string]struct{}, len(engine.Routes()))
	for _, route := range engine.Routes() {
		routes[route.Method+" "+route.Path] = struct{}{}
	}

	require.Contains(t, routes, http.MethodGet+" /api/download/unicomp/windows/count")
	require.Contains(t, routes, http.MethodPost+" /api/download/unicomp/windows")
	require.Contains(t, routes, http.MethodGet+" /api/download/windows/count")
	require.Contains(t, routes, http.MethodPost+" /api/download/windows")
}
