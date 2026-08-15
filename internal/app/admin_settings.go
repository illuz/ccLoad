package app

import (
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"strconv"

	"ccLoad/internal/model"

	"github.com/gin-gonic/gin"
)

// 配置验证常量
const (
	LogRetentionDaysMin      = 1
	LogRetentionDaysMax      = 365
	LogRetentionDaysDisabled = -1 // 永久保留
)

// AdminListSettings 获取所有配置项
// GET /admin/settings
func (s *Server) AdminListSettings(c *gin.Context) {
	settings, err := s.configService.ListAllSettings(c.Request.Context())
	if err != nil {
		log.Printf("[ERROR] AdminListSettings 失败: %v", err)
		RespondError(c, http.StatusInternalServerError, err)
		return
	}

	if settings == nil {
		settings = make([]*model.SystemSetting, 0)
	}
	for i, setting := range settings {
		if setting == nil {
			continue
		}
		settingCopy := *setting
		settingCopy.HotReload = isHotReloadableSetting(setting.Key)
		settings[i] = &settingCopy
	}
	RespondJSON(c, http.StatusOK, settings)
}

// AdminGetSetting 获取单个配置项
// GET /admin/settings/:key
func (s *Server) AdminGetSetting(c *gin.Context) {
	key := c.Param("key")
	if key == "" {
		RespondErrorMsg(c, http.StatusBadRequest, "missing setting key")
		return
	}

	// 管理接口必须返回持久化后的最新值，不能复用等待重启的运行时缓存。
	setting, err := s.configService.GetSettingFresh(c.Request.Context(), key)
	if errors.Is(err, model.ErrSettingNotFound) {
		RespondErrorMsg(c, http.StatusNotFound, fmt.Sprintf("setting not found: %s", key))
		return
	}
	if err != nil {
		log.Printf("[ERROR] AdminGetSetting key=%s 失败: %v", key, err)
		RespondError(c, http.StatusInternalServerError, err)
		return
	}

	settingCopy := *setting
	settingCopy.HotReload = isHotReloadableSetting(setting.Key)
	RespondJSON(c, http.StatusOK, &settingCopy)
}

// AdminUpdateSetting 更新配置项
// PUT /admin/settings/:key
func (s *Server) AdminUpdateSetting(c *gin.Context) {
	key := c.Param("key")
	if key == "" {
		RespondErrorMsg(c, http.StatusBadRequest, "missing setting key")
		return
	}

	var req SettingUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondErrorMsg(c, http.StatusBadRequest, fmt.Sprintf("invalid request: %v", err))
		return
	}

	// 验证值的合法性
	setting := s.configService.GetSetting(key)
	if setting == nil {
		RespondErrorMsg(c, http.StatusNotFound, fmt.Sprintf("setting not found: %s", key))
		return
	}

	if err := validateSettingValue(key, setting.ValueType, req.Value); err != nil {
		RespondErrorMsg(c, http.StatusBadRequest, fmt.Sprintf("invalid value for type %s: %v", setting.ValueType, err))
		return
	}

	// 更新配置
	if err := s.configService.UpdateSetting(c.Request.Context(), key, req.Value); err != nil {
		log.Printf("[ERROR] AdminUpdateSetting key=%s 失败: %v", key, err)
		RespondError(c, http.StatusInternalServerError, err)
		return
	}
	respondAfterSettingsUpdate(c, s, map[string]string{key: req.Value}, gin.H{
		"key":   key,
		"value": req.Value,
	})
}

// AdminResetSetting 重置配置为默认值
// POST /admin/settings/:key/reset
func (s *Server) AdminResetSetting(c *gin.Context) {
	key := c.Param("key")
	if key == "" {
		RespondErrorMsg(c, http.StatusBadRequest, "missing setting key")
		return
	}

	// 获取默认值
	setting := s.configService.GetSetting(key)
	if setting == nil {
		RespondErrorMsg(c, http.StatusNotFound, fmt.Sprintf("setting not found: %s", key))
		return
	}

	// 重置为默认值
	if err := s.configService.UpdateSetting(c.Request.Context(), key, setting.DefaultValue); err != nil {
		log.Printf("[ERROR] AdminResetSetting key=%s 失败: %v", key, err)
		RespondError(c, http.StatusInternalServerError, err)
		return
	}
	respondAfterSettingsUpdate(c, s, map[string]string{key: setting.DefaultValue}, gin.H{
		"key":   key,
		"value": setting.DefaultValue,
	})
}

// AdminBatchUpdateSettings 批量更新配置(事务保护)
// POST /admin/settings/batch
func (s *Server) AdminBatchUpdateSettings(c *gin.Context) {
	var req map[string]string
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondErrorMsg(c, http.StatusBadRequest, fmt.Sprintf("invalid request: %v", err))
		return
	}

	if len(req) == 0 {
		RespondErrorMsg(c, http.StatusBadRequest, "no settings to update")
		return
	}

	if err := validateSettingsUpdates(s, req); err != nil {
		RespondErrorMsg(c, http.StatusBadRequest, err.Error())
		return
	}

	// 批量更新(事务保护)
	if err := s.configService.BatchUpdateSettings(c.Request.Context(), req); err != nil {
		log.Printf("[ERROR] AdminBatchUpdateSettings 失败: %v", err)
		RespondError(c, http.StatusInternalServerError, err)
		return
	}

	respondAfterSettingsUpdate(c, s, req, gin.H{})
}

// AdminSaveAndRestartSettings 保存可选的设置变更，并始终触发程序重启。
// 空对象也视为有效请求，用于用户明确要求重启但没有修改设置的场景。
// POST /admin/settings/save-restart
func (s *Server) AdminSaveAndRestartSettings(c *gin.Context) {
	req := make(map[string]string)
	if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
		RespondErrorMsg(c, http.StatusBadRequest, fmt.Sprintf("invalid request: %v", err))
		return
	}

	if err := validateSettingsUpdates(s, req); err != nil {
		RespondErrorMsg(c, http.StatusBadRequest, err.Error())
		return
	}

	if len(req) > 0 {
		if err := s.configService.BatchUpdateSettings(c.Request.Context(), req); err != nil {
			log.Printf("[ERROR] AdminSaveAndRestartSettings 保存失败: %v", err)
			RespondError(c, http.StatusInternalServerError, err)
			return
		}
		s.ApplyHotReloadableSettings(req)
	}

	log.Printf("[INFO] 已保存 %d 项配置，用户请求立即重启", len(req))
	RespondJSON(c, http.StatusOK, gin.H{
		"message":    fmt.Sprintf("已保存 %d 项配置，程序将立即重启", len(req)),
		"restarting": true,
	})
	go triggerRestart()
}

func validateSettingsUpdates(s *Server, updates map[string]string) error {
	for key, value := range updates {
		setting := s.configService.GetSetting(key)
		if setting == nil {
			return fmt.Errorf("unknown setting: %s", key)
		}
		if err := validateSettingValue(key, setting.ValueType, value); err != nil {
			return fmt.Errorf("invalid value for %s: %v", key, err)
		}
	}
	return nil
}

func respondAfterSettingsUpdate(c *gin.Context, s *Server, updates map[string]string, response gin.H) {
	hasRestartRequiredSetting := false
	for key := range updates {
		if !isHotReloadableSetting(key) {
			hasRestartRequiredSetting = true
			break
		}
	}

	s.ApplyHotReloadableSettings(updates)
	if hasRestartRequiredSetting {
		log.Printf("[INFO] 已保存 %d 项配置（包含需重启项）", len(updates))
		response["message"] = fmt.Sprintf("已保存 %d 项配置，程序将立即重启", len(updates))
		RespondJSON(c, http.StatusOK, response)
		go triggerRestart()
		return
	}

	log.Printf("[INFO] 已热更新 %d 项配置", len(updates))
	response["message"] = fmt.Sprintf("已保存 %d 项配置并立即生效", len(updates))
	response["hot_reloaded"] = true
	RespondJSON(c, http.StatusOK, response)
}

// validateSettingValue 验证配置值的合法性
func validateSettingValue(key, valueType, value string) error {
	switch valueType {
	case "int":
		intVal, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("not a valid integer")
		}
		// 按配置项定义具体约束
		switch key {
		case "max_key_retries":
			if intVal < 1 {
				return fmt.Errorf("max_key_retries must be >= 1")
			}
		case "channel_check_interval_hours":
			if intVal < 0 {
				return fmt.Errorf("channel_check_interval_hours must be >= 0")
			}
		case "channel_balance_refresh_interval_seconds":
			if intVal < 0 {
				return fmt.Errorf("channel_balance_refresh_interval_seconds must be >= 0")
			}
		case "log_retention_days":
			if intVal != LogRetentionDaysDisabled && (intVal < LogRetentionDaysMin || intVal > LogRetentionDaysMax) {
				return fmt.Errorf("log_retention_days must be %d (永久) or %d-%d", LogRetentionDaysDisabled, LogRetentionDaysMin, LogRetentionDaysMax)
			}
		default:
			if intVal < -1 {
				return fmt.Errorf("value must be >= -1")
			}
		}

	case "float":
		floatVal, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return fmt.Errorf("not a valid number")
		}
		if math.IsNaN(floatVal) || math.IsInf(floatVal, 0) {
			return fmt.Errorf("must be a finite number")
		}
		switch key {
		case "model_catalog_sync_interval_hours":
			if floatVal < 0 {
				return fmt.Errorf("%s must be >= 0", key)
			}
		}

	case "bool":
		if value != "true" && value != "false" && value != "1" && value != "0" {
			return fmt.Errorf("must be true/false or 1/0")
		}

	case "duration":
		intVal, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("duration must be an integer (seconds)")
		}
		if intVal < 0 {
			return fmt.Errorf("duration must be >= 0 (0 = disabled)")
		}

	case "auth_token_id":
		intVal, err := strconv.ParseInt(value, 10, 64)
		if err != nil || intVal < 0 {
			return fmt.Errorf("auth token ID must be a non-negative integer")
		}

	case "string":
		switch key {
		case "log_channel_click_action":
			if value != "edit" && value != "navigate" {
				return fmt.Errorf("log_channel_click_action must be edit or navigate")
			}
		}

	default:
		return fmt.Errorf("unknown value type: %s", valueType)
	}

	return nil
}

// RestartFunc 重启函数（由 main 包注入，避免循环依赖）
var RestartFunc func()

// triggerRestart 触发程序重启
// 依赖优雅关闭语义：触发 SIGTERM 后，HTTP 服务器应完成当前请求再退出。
func triggerRestart() {
	log.Print("[INFO] 配置变更触发重启...")

	if RestartFunc == nil {
		log.Printf("[ERROR] RestartFunc 为空，重启已跳过")
		return
	}
	RestartFunc()
}
