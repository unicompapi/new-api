package controller

import (
	"net/http"

	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

const (
	windowsDownloadKey = "unicomp-ai-windows"
	windowsDownloadURL = "https://picture.unicompapi.com/unicompapi/seedance/2026/UniComp%20AI%20Setup%201.0.0.exe"
)

func GetWindowsDownloadCount(c *gin.Context) {
	count, err := model.GetDownloadCount(windowsDownloadKey)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "读取下载次数失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": gin.H{"count": count}})
}

func DownloadWindows(c *gin.Context) {
	if _, err := model.IncrementDownloadCount(windowsDownloadKey); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "记录下载次数失败，请稍后重试"})
		return
	}
	c.Redirect(http.StatusFound, windowsDownloadURL)
}
