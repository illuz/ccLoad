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

## 2026-07-27 selected upstream batch

Upstream head checked: `caidaoli/ccLoad` `upstream/master` at `61568753` (`v3.13.1`, `fix: gracefully degrade unsupported alpha search`).
Local safety branch before work: `backup-before-upstream-selected-20260727-235535`.

Upstream history was rewritten after the previously reviewed `47c86fc`; that commit is no longer an ancestor of current `upstream/master`. The items below were therefore compared by patch and behavior rather than by a simple revision range.

| Review item | Decision | Status | Local commits | Notes for future checks |
| --- | --- | --- | --- | --- |
| Low-risk correctness fixes | Accept | Merged | `25e24078`, `f0b27066`, `21afd3fb`, `592cb8a7`, `0537ff01`, `b2167c72`, `f8212e15` | Covers debug-log clipboard fallback, cache-write/reasoning usage parsing, reachable token editor content, disabled-key model fetch filtering, replica restore/startup timeout handling, and Docker Compose password interpolation. |
| Responses semantic stream completion | Accept | Merged with local stream paths adapted | `2b13bc42` | Stops reading after protocol-level completion without depending only on transport EOF. Review only later completion-state fixes. |
| Model-scoped cooldowns and status UI | Accept | Merged and adapted to local retry/SQL layers | `4cbbc28f`, `a5bb4e21`, `b7566641`, `6a42b64b`, `73ccf289`, `dab71445`, `309882b7`, `a9a82e03` | Model-specific 404/429/5xx failures no longer unnecessarily cool an entire channel. Local retry semantics, persistent cooldown storage, model stats, and clearing all cooldowns on save are integrated. |
| Runtime pricing and models.dev catalog | Accept | Merged with network sync disabled by default | `e7ad5341`, `a98d3ba9`, `bcf57fe2`, `f73e52c3`, `4059e4db`, `752361a7`, `fa9a5078`, `51180a8f`, `6abc9854`, `0dc4b201`, `4148f144` | Runtime prices and common models can come from the synchronized catalog. `model_catalog_sync_interval_hours=0` remains the default, so no new outbound network request occurs unless explicitly enabled. |
| Channel-relative TTFB priority penalty | Accept | Merged with local selector behavior retained | `f590bebd`, `e3bb2289` | Preserves local URL EWMA and input-token priority bonus. TTFB scoring defaults off and only applies when health sorting is also enabled. |
| Token channel allow/deny policies | Accept | Reimplemented for local token groups and inheritance | `64dcdfa8` | Based on upstream `54028db1`, extended to Token Groups, effective-value JSON, SQLite/MySQL/hybrid restore paths, Gemini/OpenAI model visibility, and the local editor. Empty lists are always unrestricted; inheriting channels inherits both IDs and mode. |

## 2026-07-27 explicitly not selected

Do not recommend these again unless the user asks for them or a future selected fix requires them:

- PostgreSQL support (`5c98af1e`) because this fork's storage and restore guarantees are built around SQLite, MySQL, and hybrid replication; this needs a separate architecture decision.
- Token-scoped read-only dashboard/session work (`562da161` through `f73419ec`) because it adds a second authorization surface and should receive a dedicated security review.
- Model fingerprint calibration/comparison (`598970bc` through `486e51ed`) because it is a large independent storage, job-management, API, and UI subsystem.
- Codex/Responses WebSocket support and wholesale CLIProxy core sync (`82d23a07` through `7d32c027`, plus the related protocol sync commits) because the protocol and session-lifecycle blast radius is too large for a selective upstream merge.
- Bulk model-name normalization/source-prefix removal (`4f601a80`, `1f274c55`) because these are destructive administrative transformations rather than proxy correctness fixes.
- Channel cooldown probe rules (`44c0e370`, `4ba9068e`, `76b36f33`), the nine-setting migration (`e7b23e96`), and standard-cost detail UI (`5a746111`) because they are useful but not prerequisites for the selected routing and accounting work.

For the next upstream check, treat `61568753` as the content-review checkpoint. Verify ancestry first; if upstream has been rewritten again, compare patches/features rather than using `61568753..upstream/master` blindly.
