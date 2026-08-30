package model

import (
	"strconv"
	"strings"
)

// ThinkingSuffixMode describes how a recognized model suffix controls thinking.
type ThinkingSuffixMode uint8

const (
	ThinkingSuffixNone ThinkingSuffixMode = iota + 1
	ThinkingSuffixAuto
	ThinkingSuffixLevel
	ThinkingSuffixBudget
)

// ThinkingSuffixConfig is the normalized value from a model suffix such as
// "(high)", "(auto)", or "(8192)".
type ThinkingSuffixConfig struct {
	Mode   ThinkingSuffixMode
	Level  string
	Budget int
}

// ParseThinkingSuffix recognizes a trailing thinking modifier. Unknown suffixes
// remain part of the model name because an upstream may legitimately use them.
func ParseThinkingSuffix(modelName string) (base string, cfg ThinkingSuffixConfig, ok bool) {
	open := strings.LastIndex(modelName, "(")
	if open < 0 || !strings.HasSuffix(modelName, ")") {
		return modelName, ThinkingSuffixConfig{}, false
	}
	base = strings.TrimSpace(modelName[:open])
	raw := strings.TrimSpace(modelName[open+1 : len(modelName)-1])
	if base == "" || raw == "" {
		return modelName, ThinkingSuffixConfig{}, false
	}

	switch strings.ToLower(raw) {
	case "none":
		return base, ThinkingSuffixConfig{Mode: ThinkingSuffixNone}, true
	case "auto", "-1":
		return base, ThinkingSuffixConfig{Mode: ThinkingSuffixAuto, Budget: -1}, true
	case "minimal", "low", "medium", "high", "xhigh", "max":
		return base, ThinkingSuffixConfig{Mode: ThinkingSuffixLevel, Level: strings.ToLower(raw)}, true
	}

	budget, err := strconv.Atoi(raw)
	if err != nil || budget < 0 {
		return modelName, ThinkingSuffixConfig{}, false
	}
	if budget == 0 {
		return base, ThinkingSuffixConfig{Mode: ThinkingSuffixNone}, true
	}
	return base, ThinkingSuffixConfig{Mode: ThinkingSuffixBudget, Budget: budget}, true
}

// RoutingModelName strips a recognized thinking suffix for routing, access
// checks, cooldown keys, logs, and the model name sent upstream.
func RoutingModelName(modelName string) string {
	base, _, ok := ParseThinkingSuffix(modelName)
	if ok {
		return base
	}
	return modelName
}
