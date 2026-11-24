package api

import (
	"net/http"
	"runtime"
	"time"

	"chrelyonly-email-server-go/pkg/models"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

var startTime = time.Now()

type SystemHandler struct {
	db *gorm.DB
}

func NewSystemHandler(db *gorm.DB) *SystemHandler {
	return &SystemHandler{db: db}
}

func (h *SystemHandler) Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":    "ok",
		"service":   "chrelyonly-mail-server",
		"timestamp": time.Now().Format(time.RFC3339),
	})
}

func (h *SystemHandler) Status(c *gin.Context) {
	var userCount int64
	var emailCount int64
	var mStats runtime.MemStats
	runtime.ReadMemStats(&mStats)

	h.db.Model(&models.User{}).Count(&userCount)
	h.db.Model(&models.Email{}).Count(&emailCount)

	c.JSON(http.StatusOK, gin.H{
		"status":          "running",
		"uptime":          time.Since(startTime).String(),
		"users_count":     userCount,
		"email_count":     emailCount,
		"memory_alloc_mb": mStats.Alloc / 1024 / 1024,
		"goroutines":      runtime.NumGoroutine(),
	})
}
