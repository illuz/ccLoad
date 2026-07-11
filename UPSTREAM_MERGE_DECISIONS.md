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

## 2026-07-11 selected upstream follow-up batch

Upstream range reviewed: `b038293..47c86fc` (latest checked upstream commit: `47c86fc`, 2026-07-10).
Local safety branch before work: `backup-before-upstream-selected-20260711-012625`.

| Review item | Decision | Status | Upstream commits merged locally | Notes for future checks |
| --- | --- | --- | --- | --- |
| 1. Model pricing follow-ups | Accept | Merged | `7103861` GLM-5.2; `dffc145` tiered MiniMax-M3; `3b32cee` Cerebras; `4c3346b` Grok pricing | Treat as completed pricing maintenance. Only review later pricing changes after `4c3346b`. |
| 2. Persist token effective cost | Accept | Merged with local Codex Guard billing integration retained | `676145d` | `effective_cost_usd` stores channel-multiplied cost; local `isBillable` semantics, daily usage, and alert checks were preserved during conflict resolution. Review only later follow-up fixes. |
| 6. Unified timing color rendering | Accept | Merged with local token summary/priority UI retained | `3683278` | Shared first-byte/total-duration color functions now apply across relevant views. Review only later fixes. |
| 7. Codex GPT-5.6 common models | Accept | Merged | `47c86fc` | Adds `gpt-5.6-sol`, `gpt-5.6-luna`, and `gpt-5.6-terra` to the Codex common-model picker. |

## 2026-07-11 explicitly not selected

Do not recommend these again unless the user asks or a selected follow-up depends on them:

- Active-request tab alert and its favicon compatibility fixes (`f0b9b27`, `01b385b`, `d3b8cf1`, `6bbad7d`).
- Channel editor table/adaptive layout work (`7b32fb4`, `5b86be5`).

When checking later upstream changes, use `47c86fc` as the reviewed baseline, while retaining earlier decisions above.
