package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

const defaultDebugAnalysisDir = "data/debug-analysis"

func debugAnalysisDir() string {
	if dir := strings.TrimSpace(os.Getenv("CCLOAD_DEBUG_ANALYSIS_DIR")); dir != "" {
		return dir
	}
	return defaultDebugAnalysisDir
}

func debugAnalysisPath(logID int64) string {
	return filepath.Join(debugAnalysisDir(), fmt.Sprintf("%d.json", logID))
}

// HandleGetDebugLogAnalysis 获取独立分析器生成的 Debug 日志分析结果。
// GET /admin/debug-log-analysis/:log_id
func (s *Server) HandleGetDebugLogAnalysis(c *gin.Context) {
	logIDStr := c.Param("log_id")
	logID, err := strconv.ParseInt(logIDStr, 10, 64)
	if err != nil || logID <= 0 {
		RespondErrorMsg(c, http.StatusBadRequest, "invalid log_id")
		return
	}

	path := debugAnalysisPath(logID)
	data, err := os.ReadFile(path) //nolint:gosec // path is constrained to numeric log_id under configured analysis dir
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			RespondErrorWithData(c, http.StatusNotFound, "analysis not found", gin.H{
				"log_id": logID,
				"path":   path,
			})
			return
		}
		RespondError(c, http.StatusInternalServerError, err)
		return
	}

	var payload any
	if err := json.Unmarshal(data, &payload); err != nil {
		RespondErrorMsg(c, http.StatusInternalServerError, "analysis json is invalid")
		return
	}

	RespondJSON(c, http.StatusOK, payload)
}
