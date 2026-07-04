package app

import (
	"fmt"
	"net/http"
	"strings"

	"ccLoad/internal/protocol"
	"ccLoad/internal/util"
)

const codexGuardMaxBufferedBytes = 32 * 1024 * 1024

var defaultCodexGuardReasoningEquals = map[int]struct{}{
	516:  {},
	1034: {},
	1552: {},
}

type codexGuardVerdict struct {
	Triggered       bool
	Reason          string
	ReasoningTokens int
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
	if _, ok := defaultCodexGuardReasoningEquals[res.ReasoningTokens]; !ok {
		return codexGuardVerdict{}
	}
	reason := fmt.Sprintf("codex_guard reasoning_tokens=%d", res.ReasoningTokens)
	return codexGuardVerdict{
		Triggered:       true,
		Reason:          reason,
		ReasoningTokens: res.ReasoningTokens,
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
