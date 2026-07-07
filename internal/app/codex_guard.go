package app

import (
	"fmt"
	"net/http"
	"strings"

	"ccLoad/internal/protocol"
	"ccLoad/internal/util"
)

const (
	codexGuardMaxBufferedBytes = 32 * 1024 * 1024
	// codexGuardMaxRetries 是 Guard 命中后的最大重试次数，不包含首次请求。
	// 即最多 1 次首次请求 + 4 次 guard retry。
	codexGuardMaxRetries = 4
	// codexGuardReducedBudgetThresholdBytes 是“接收超过 500KiB 后降低本请求系列重试预算”的阈值。
	codexGuardReducedBudgetThresholdBytes = 500 * 1024
	// codexGuardReducedBudgetThresholdSeconds 是“单次 Guard attempt 耗时超过 50s 后降低本请求系列重试预算”的阈值。
	codexGuardReducedBudgetThresholdSeconds = 50.0
	// codexGuardReducedBudgetMaxRetries 是触发大响应/慢 attempt 后，本请求系列的最大重试次数，不包含首次请求。
	codexGuardReducedBudgetMaxRetries = 1
)

type codexGuardVerdict struct {
	Triggered        bool
	AllowPassthrough bool
	Reason           string
	ReasoningTokens  int
}

func shouldApplyCodexReasoningGuard(reqCtx *requestContext, channelType string) bool {
	if reqCtx == nil || !reqCtx.codexGuardEnabled {
		return false
	}
	if reqCtx.clientProtocol == protocol.Codex || reqCtx.upstreamProtocol == protocol.Codex ||
		reqCtx.transformPlan.ClientProtocol == protocol.Codex || reqCtx.transformPlan.UpstreamProtocol == protocol.Codex {
		return true
	}
	return util.NormalizeChannelType(channelType) == util.ChannelTypeCodex ||
		strings.EqualFold(strings.TrimSpace(channelType), string(protocol.Codex))
}

func codexGuardReasoningMatched(reasoningTokens int) bool {
	return reasoningTokens >= 516 && (reasoningTokens+2)%518 == 0
}

func evaluateCodexReasoningGuard(reqCtx *requestContext, res *fwResult) codexGuardVerdict {
	if reqCtx == nil || res == nil || !reqCtx.codexGuardEnabled {
		return codexGuardVerdict{}
	}
	if reqCtx.clientProtocol != protocol.Codex && reqCtx.upstreamProtocol != protocol.Codex &&
		reqCtx.transformPlan.ClientProtocol != protocol.Codex && reqCtx.transformPlan.UpstreamProtocol != protocol.Codex {
		return codexGuardVerdict{}
	}
	if res.Status < 200 || res.Status >= 300 {
		return codexGuardVerdict{}
	}
	if !codexGuardReasoningMatched(res.ReasoningTokens) {
		return codexGuardVerdict{}
	}
	reason := fmt.Sprintf("codex_guard reasoning_tokens=%d match=518n-2", res.ReasoningTokens)
	return codexGuardVerdict{
		Triggered:        true,
		AllowPassthrough: reqCtx.codexGuardPassthroughOnLastAttempt,
		Reason:           reason,
		ReasoningTokens:  res.ReasoningTokens,
	}
}

func codexGuardBlockedResult(res *fwResult, verdict codexGuardVerdict) *fwResult {
	if res == nil {
		res = &fwResult{}
	}
	body := []byte(fmt.Sprintf(`{"error":{"message":"Codex reasoning guard triggered: reasoning_tokens=%d","type":"upstream_error","code":"codex_reasoning_guard"}}`, verdict.ReasoningTokens))
	blocked := *res
	blocked.Status = util.StatusCodexReasoningGuard
	blocked.Header = http.Header{"Content-Type": []string{"application/json; charset=utf-8"}}
	blocked.Body = body
	blocked.StreamDiagMsg = verdict.Reason
	blocked.ResponseCommitted = false
	return &blocked
}

func ensureCodexGuardTraceID(reqCtx *proxyRequestContext) string {
	if reqCtx == nil {
		return ""
	}
	if reqCtx.codexGuardTraceID == "" {
		reqCtx.codexGuardTraceID = util.NewUUIDv4()
	}
	return reqCtx.codexGuardTraceID
}

func observeCodexGuardResponse(reqCtx *proxyRequestContext, res *fwResult, durationSeconds float64) {
	if reqCtx == nil || res == nil {
		return
	}
	if res.BytesReceived > codexGuardReducedBudgetThresholdBytes ||
		durationSeconds > codexGuardReducedBudgetThresholdSeconds {
		reqCtx.codexGuardReducedBudget = true
	}
}

func codexGuardMaxRetriesForRequest(reqCtx *proxyRequestContext) int {
	if reqCtx != nil && reqCtx.codexGuardReducedBudget {
		return codexGuardReducedBudgetMaxRetries
	}
	return codexGuardMaxRetries
}

func markCodexGuardExhausted(reqCtx *proxyRequestContext) {
	if reqCtx != nil {
		reqCtx.codexGuardExhausted = true
	}
}

func markCodexGuardPassthroughResult(res *fwResult, verdict codexGuardVerdict) {
	if res == nil || !verdict.AllowPassthrough {
		return
	}
	res.CodexGuardPassthrough = true
}
