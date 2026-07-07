# Upstream merge decisions

This file records selected upstream feature decisions so future upstream checks do not re-evaluate already-decided items from the same batch.

## 2026-07-08 selected upstream batch

Upstream baseline checked: `caidaoli/ccLoad` `upstream/master` at `b038293` (`feat: add GPT-5.6 pricing`).
Local safety branch before work: `backup-before-selected-upstream-20260708-015357`.

| ID from review | Decision | Status | Upstream commits merged locally | Notes for future checks |
| --- | --- | --- | --- | --- |
| 1. New model pricing | Accept | Merged | `c8985fa` Claude Sonnet 5 pricing; `b038293` GPT-5.6 pricing/cache creation handling | Do not re-evaluate this feature. Only consider later upstream follow-up commits if they change pricing after `b038293`. |
| 2. Gemini invalid key classification | Accept | Merged | `ef5deec` | Do not re-evaluate this feature. |
| 3. Codex/thinking effort fixes | Accept | Merged | `103c3b0`, `1e96dda` | Do not re-evaluate the xhigh/max/reasoning preservation decision. Future follow-up fixes may still be considered. |
| 6. API key notes | Accept | Merged | `73894b5` | Do not re-evaluate this feature. Preserve local channel fields when resolving future conflicts. |

## Current deferred/not selected items from the same upstream review

These were not selected in this batch; keep them out unless explicitly requested or they become prerequisites for a selected bug fix:

- Runtime-protocol auth header fix (`861fd40`) — recommended but not selected in this batch.
- Codex tool search retry fixes (`2384cf5`, `a1af9df`) — useful for Codex/anyrouter but not selected in this batch.
- Markdown/debug/tool-call rendering improvements (`234a73a`, `220a379`, `2963ccf`, `ca74e93`, `d67385a`, `70eac2d`) — useful but front-end conflict-heavy; defer.
- Automatic binary updates (`4900054`, `b5cf665`, `6e312cf`, `8481dfa`) — defer/avoid for this fork unless an explicit custom update strategy is desired.

## Future upstream-check instruction

When asked to check upstream again, use this file first:

1. Treat merged/accepted rows above as already decided and completed.
2. Compare upstream from `b038293` onward for genuinely new items or follow-up fixes.
3. Do not spend analysis time re-arguing the accepted decisions above.
