package app

import (
	"net/http"
	"time"

	"ccLoad/internal/model"
	"ccLoad/internal/util"

	"github.com/gin-gonic/gin"
)

const proxyRequestIDContextKey = "ccLoad.proxyRequestID"

func ensureProxyRequestID(c *gin.Context) string {
	if c == nil {
		return util.NewUUIDv4()
	}
	if value, ok := c.Get(proxyRequestIDContextKey); ok {
		if requestID, ok := value.(string); ok && requestID != "" {
			return requestID
		}
	}
	requestID := util.NewUUIDv4()
	c.Set(proxyRequestIDContextKey, requestID)
	return requestID
}

// logAPIAuthRejections records requests stopped by RequireAPIAuth before they
// can reach HandleProxyRequest. RequireAPIAuth supplies the admin-facing
// diagnostic message.
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
	duration := time.Since(startTime).Seconds()
	s.AddLogAsync(&model.LogEntry{
		Time:                  model.JSONTime{Time: startTime},
		RequestID:             ensureProxyRequestID(c),
		Model:                 modelName,
		LogSource:             model.LogSourceProxy,
		StatusCode:            statusCode,
		Message:               message,
		Duration:              duration,
		EndToEndFirstByteTime: duration,
		IsStreaming:           isStreaming,
		AuthTokenID:           tokenIDInt64,
		ClientIP:              c.ClientIP(),
		ThinkingEffort:        thinkingEffort,
	})
}
