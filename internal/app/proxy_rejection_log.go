package app

import (
	"net/http"
	"time"

	"ccLoad/internal/model"

	"github.com/gin-gonic/gin"
)

// logAPIAuthRejections records requests stopped by RequireAPIAuth before they
// can reach HandleProxyRequest. It deliberately does not inspect or persist the
// submitted credential.
func (s *Server) logAPIAuthRejections() gin.HandlerFunc {
	return func(c *gin.Context) {
		fallbackStart := time.Now()
		c.Next()

		value, rejected := c.Get(apiAuthRejectionContextKey)
		if !rejected {
			return
		}
		message, _ := value.(string)
		if message == "" {
			message = "authorization rejected"
		}

		modelName := extractModelFromPath(c.Request.URL.Path)
		if modelName == "" && c.Request.Method == http.MethodGet {
			modelName = "*"
		}
		s.recordProxyRejection(
			c,
			proxyTimingStartTime(c, fallbackStart),
			modelName,
			c.Writer.Status(),
			message,
			false,
			"",
		)
	}
}

func (s *Server) recordProxyRejection(
	c *gin.Context,
	startTime time.Time,
	modelName string,
	statusCode int,
	message string,
	isStreaming bool,
	thinkingEffort string,
) {
	// A few narrow unit tests construct a minimal Server without persistence.
	if s == nil || s.logService == nil || c == nil {
		return
	}
	if startTime.IsZero() {
		startTime = time.Now()
	}

	tokenID, _ := c.Get("token_id")
	tokenIDInt64, _ := tokenID.(int64)
	s.AddLogAsync(&model.LogEntry{
		Time:           model.JSONTime{Time: startTime},
		Model:          modelName,
		LogSource:      model.LogSourceProxy,
		StatusCode:     statusCode,
		Message:        message,
		Duration:       time.Since(startTime).Seconds(),
		IsStreaming:    isStreaming,
		AuthTokenID:    tokenIDInt64,
		ClientIP:       c.ClientIP(),
		ThinkingEffort: thinkingEffort,
	})
}
