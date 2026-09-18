package controller

import (
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

const (
	windowsDownloadKey        = "unicomp-ai-windows"
	windowsDownloadURL        = "https://picture.unicompapi.com/unicompapi/seedance/2026/UniComp%20AI%20Setup%201.0.0.exe"
	uniCompWindowsDownloadKey = "unicomp-desktop-windows"
	uniCompWindowsDownloadURL = "https://picture.unicompapi.com/unicompapi/desktop/unicomp/v1.0.0/UniComp-1.0.0-setup-x64.exe"
)

func GetWindowsDownloadCount(c *gin.Context) {
	getDownloadCount(c, windowsDownloadKey)
}

func DownloadWindows(c *gin.Context) {
	download(c, windowsDownloadKey, windowsDownloadURL)
}

func GetUniCompWindowsDownloadCount(c *gin.Context) {
	getDownloadCount(c, uniCompWindowsDownloadKey)
}

func DownloadUniCompWindows(c *gin.Context) {
	download(c, uniCompWindowsDownloadKey, uniCompWindowsDownloadURL)
}

func getDownloadCount(c *gin.Context, key string) {
	count, err := model.GetDownloadCount(key)
	if err != nil {
		common.SysError("failed to read download count for " + key + ": " + err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": common.TranslateMessage(c, i18n.MsgDatabaseError),
		})
		return
	}
	common.ApiSuccess(c, gin.H{"count": count})
}

// download records the hit and redirects to the installer. The counter is
// best-effort telemetry, so a write failure is logged but must not block the
// download itself: the redirect is the user-facing feature.
func download(c *gin.Context, key string, targetURL string) {
	if _, err := model.IncrementDownloadCount(key); err != nil {
		common.SysError("failed to record download count for " + key + ": " + err.Error())
	}
	c.Redirect(http.StatusFound, targetURL)
}
