package app

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strconv"
	"strings"

	"ccLoad/internal/debuganalysis"

	"github.com/gin-gonic/gin"
)

const defaultDebugAnalysisDir = "data/debug-analysis"

func debugAnalysisDir() string {
	if dir := strings.TrimSpace(os.Getenv("CCLOAD_DEBUG_ANALYSIS_DIR")); dir != "" {
		return dir
	}
	return defaultDebugAnalysisDir
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

	path, err := debuganalysis.FindOutputPath(debugAnalysisDir(), logID)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			RespondErrorWithData(c, http.StatusNotFound, "analysis not found", gin.H{
				"log_id": logID,
				"path":   debugAnalysisDir(),
			})
			return
		}
		RespondError(c, http.StatusInternalServerError, err)
		return
	}
	data, err := debuganalysis.ReadOutput(c.Request.Context(), path)
	if err != nil {
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
