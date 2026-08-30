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

## 2026-08-03 selected upstream batch

Upstream head checked: `caidaoli/ccLoad` `upstream/master` at `898552b2` (`v4.5.0`, `fix: 模型刷新时去重重定向目标`).
Local safety branch before work: `backup-before-upstream-selected-20260803-161023`.

The upstream v4 protocol refactor is not present in this fork, so the selected changes were ported by behavior and covered with local tests instead of being cherry-picked wholesale.

| Review item | Decision | Status | Upstream source commits | Local commits and adaptation notes |
| --- | --- | --- | --- | --- |
| GPT-5.6 pricing corrections | Accept | Ported | `8bf4b605`, `5defe016` | `1d434326`, `6395f8e3`; adds the 272K tiers, cache-read tier accounting, and current Terra/Luna rates without replacing the local runtime catalog. |
| Clear cooldowns when enabling a channel | Accept | Ported | `8d9dd4ec` | `cddc67cd`; clears channel, key, URL, and model cooldown state through the local admin and cache layers. |
| Codex Fast billing | Accept | Ported | `532de95f` | `76fea8e5`; bills the actual upstream model and preserves the local billable-log semantics. |
| Per-model enable/disable | Accept | Ported | `afb9b934` | `7993e532`; adds persistence, routing exclusion, admin UI controls, and model refresh preservation for the local SQLite/MySQL storage model. |
| Model refresh redirect-target deduplication | Accept | Ported | `898552b2` | `70c2c4e8`; deduplicates against both exposed model names and upstream redirect targets in the backend and editor. |
| Dynamic log status-code filter | Accept | Ported | `7b943df3` | `63eceafe`; uses the local `/admin/models` statistics contract rather than upstream's v4 dashboard bootstrap. |
| Actual service-tier multiplier display | Accept | Ported | `c0be3960` | `b55e2f1b`; accounting returns a transient `service_tier_multiplier` for the actual model, including GPT Fast/Flex and Claude Opus Fast rates. |
| Upstream connection reuse limit | Accept | Ported | `6c49386a` | `83acf468`; applies to this fork's HTTP/1.1, HTTP/2, and channel-proxy pools. Setting `upstream_connection_reuse_limit_seconds` defaults to `0` and takes effect after restart. This does not claim support for the absent Responses WebSocket pool. |
| One-click channel import | Selective adaptation | Ported | `38c4126c` (reviewed together with the earlier protocol behavior in `2dd34360`) | `a77f9afc`; retains the existing local quick-create workflow with multiple keys, priorities, channel/auth-token groups, source-channel model copying, and direct backend creation. Adds broader JSON/environment/Bearer parsing, terminal `/v1` removal, and atomic model discovery when no model source is supplied. Protocol remains an explicit channel-type choice because a successful Models API call does not prove the chat protocol. |

For the next upstream check, use `898552b2` as the content-review checkpoint. Do not re-propose the full `38c4126c` UI or its OpenAI-to-Anthropic Models API probing; only review later fixes that provide a reliable protocol capability signal or improve the retained local quick-create workflow.

## 2026-08-06 upstream review

Upstream range reviewed: `898552b2..f35bdfde`. Latest checked upstream commit: `f35bdfde` (`v4.6.3-beta.3`, `fix(storage): restore nullable Codex credentials`).

The selected fixes were ported by behavior because this fork retains its string-based exact-URL marker, local channel type manager, channel groups, balance display, fixed-price fields, and request-delay setting.

| Review item | Decision | Status | Upstream source commit | Local adaptation notes |
| --- | --- | --- | --- | --- |
| Restore full URL checkbox state | Reject | Not merged (equivalent already present) | `b8f6ffc7` | Keep the pre-existing local exact-URL marker renderer from `71381f70`; do not merge the upstream commit. |
| Unified channel editor snapshot and URL request statistics | Reject | Not merged (equivalent already present) | `3879fb5b` | Keep the pre-existing local `/admin/channels/:id/editor` snapshot and URL statistics implementation from `71381f70`; do not merge the upstream commit. |

## 2026-08-06 explicitly not selected

The two overlapping fixes above are also explicitly not merged; their equivalent local behavior is retained. Every other upstream change in `898552b2..f35bdfde` is explicitly not selected. This includes the release/update automation, retry and preferred-session behavior, additional channel editor/import work, automatic protocol preference, CLIProxy core sync, debug-log cleanup, and the Codex OAuth/credential/quota feature series.

Do not recommend those rejected changes again unless the user explicitly asks for one or a later accepted fix strictly depends on it. For the next upstream check, use `f35bdfde` as the content-review checkpoint and only review commits after it; verify ancestry first if upstream history is rewritten again.

## 2026-08-17 selected upstream batch

Upstream range reviewed: `f35bdfde..7815e182`. Latest checked upstream commit: `7815e182` (`v4.6.34-beta.2`, `fix(oauth): track quota cost per upstream window slot`).
Local safety branch before work: `backup-before-upstream-selected-20260817-152632`.

The accepted changes below were ported by behavior because this fork retains its local protocol, storage, logging, routing, and API-key test layers. OAuth-, Codex-, and xAI-specific branches from the upstream image-test implementation were intentionally excluded.

| Upstream source commit | Decision | Status | Local adaptation notes |
| --- | --- | --- | --- |
| `5f8e3eb6` context-overflow client error | Accept | Ported | Returns oversized-context failures as client errors so retries and cooldowns do not treat them as upstream outages. |
| `9364cede` interrupted stream status | Accept | Ported | Classifies interrupted streams as status 599. |
| `38e01749` stream error semantics | Accept | Ported | Preserves the distinction between client cancellation (499) and upstream timeout/interruption (598), including clean SSE EOF after the first-content timer cancels a request. |
| `3fbcf9e2` interrupted upstream stream status | Accept | Ported | Covers additional upstream interruption paths with status 599. |
| `00b17b5f` Anthropic `message_stop` | Accept | Ported | Treats `message_stop` as semantic stream completion without waiting for transport EOF. |
| `010a1e4d` quota reset cooldown | Accept | Ported | Uses precise upstream quota-reset timestamps when calculating cooldowns. |
| `09edaf95` Anthropic rate-limit reset | Accept | Ported | Parses Anthropic reset headers and retains the exact recovery time. |
| `d7d3ff62` clipboard and selection feedback | Accept | Ported | Corrects admin copy/selection feedback without replacing the local UI helpers. |
| `94d26ee2` direct TCP pre-probe removal | Accept | Ported | Removes the separate URL TCP probe and lets real upstream attempts drive fallback and health state. |
| `f6130b75` Gemini thinking levels | Accept | Ported | Normalizes Gemini thinking-level values through the local protocol request builder. |
| `1330ae72` model-test key selection | Accept | Ported | Adds API-key selection to channel and model tests while preserving explicit local key indexes and cooldown semantics. |
| `2e942bb8` selected-channel export | Accept | Ported | Adds CSV export for selected channels to the existing batch menu, with focused browser-side tests. |
| `4ef503f0` client protocol analytics | Accept | Ported | Persists client protocol and exposes it in log filters, statistics, trends, and grouped analytics. |
| `34f4ee51` comprehensive runtime metrics | Accept | Ported | Adds admin runtime metrics for HTTP traffic, active requests, logging, and hybrid-storage queues. |
| `256b5da8` process resource metrics | Accept | Ported | Adds uptime, CPU, RSS, heap, GC, goroutine metrics, and three-second refresh while the status dialog is open. |
| `b4b7c3f7` image-generation model test | Accept | Ported selectively | Adds native Images API and Chat Completions image tests for API-key channels, including redirects, custom rules, URL fallback, limits, response normalization, UI controls, and tests. OAuth/Codex/xAI branches were not imported. |
| `fcb88789` image prompt persistence | Accept | Ported | Persists the image prompt immediately in browser storage and restores it on page load. |

## 2026-08-17 explicitly not selected

| Upstream source commit | Decision | Notes for future checks |
| --- | --- | --- |
| `b435c2e3` startup critical-state ordering | Reject | Do not port unless a later accepted startup fix strictly depends on it. |
| `629ebb49` token channel-restriction consistency | Reject | Preserve the fork's existing token/group restriction behavior. |
| `bb274bb6` custom token-expiry timezone | Reject | Keep the current token-expiry editing behavior. |
| `51a56a33` unchanged token-expiry precision | Reject | Keep the current token-expiry editing behavior. |
| `1bce74c3` channel proxy and availability windows | Reject | Do not port upstream's combined implementation. Preserve this fork's existing `ProxyURL` behavior and do not add availability windows. |

For the next upstream check, use `7815e182` as the content-review checkpoint. Do not re-propose the five rejected commits above or the earlier rejected `b8f6ffc7`/`3879fb5b` fixes unless explicitly requested or required by a later accepted fix; verify ancestry first if upstream history is rewritten again.

## 2026-08-30 selected upstream batch

Upstream baseline checked: `caidaoli/ccLoad` `upstream/master` at `c1a5b85e` (`v4.9.1-beta.4`). The history after `7815e182` contains several upstream-only OAuth, management-account, Cursor/Zed, and WebSocket subsystems; only the items explicitly selected below are in scope for this fork.
Local safety branch before work: `backup-before-upstream-selected-20260830-continue`.

The selected changes are being ported by behavior rather than cherry-picking. This fork has different request translation, retry, storage, token-limit, and channel-capability layers, and the existing uncommitted worktree changes must remain intact.

| Upstream source commit(s) | Decision | Status | Local adaptation notes |
| --- | --- | --- | --- |
| `a5814c10`, `9ed11fe1`, `c1a5b85e` | Accept | Ported | Align Anthropic Fast pricing and charge OpenAI/Codex `ultrafast`/`auto` using the actual upstream service tier while retaining local billing and multiplier semantics. Implemented in the current worktree. |
| `ee0777ea` | Accept | Ported | Accept string-valued Responses `input` in the Codex translator without replacing the local request model types. Implemented in the current worktree. |
| `e69c5d33` | Accept | Ported | Preserve and precisely merge Anthropic beta header tokens across native, transformed, admin-test, and Z.ai paths. Implemented in the current worktree. |
| `f123e8d9` | Accept | Ported | Suppress the redirect badge when requested and actual model names are only prefix/suffix variants, while retaining the local upstream-mismatch badge. Implemented in the current worktree. |
| `0e51ebaf` | Accept | Ported | Render model cooldown recovery beyond 48 hours as days plus hours in the existing channel status UI. Implemented in the current worktree. |
| `11b5a985`, `375f4e73` | Accept | Ported | Integrate Responses input/SSE recovery and retry behavior with the local retry chain, replay rules, and channel failover. Implemented in the current worktree and covered by focused retry/replay tests. |
| `fb8972d9`, `f1216cf1` | Accept | Ported | Keep Responses metadata in upstream TTFB accounting without committing metadata-only events to the downstream stream. Implemented in the current worktree and covered by focused metadata/TTFB tests. |
| `9f4736ca` | Accept | Ported | Add a monthly API-token cost limit and reconcile it with the local daily, override, and triple-limit rules. Implemented in the current worktree. |
| `d1282a32` | Accept | Ported | Upgrade the locally generated Codex client fingerprint to the upstream `0.148.0` identity where applicable. Implemented in the current worktree. |
| `a804ba8d` | Accept | Ported | Treat recognized thinking suffixes as routing modifiers, strip them from upstream model identity, and apply protocol-specific thinking fields. Implemented in the current worktree. |
| `7015cb2c` | Accept | Ported | Track pending capability re-probes only for explicitly configured `upstream` additional protocols, skip those URLs while waiting, and expose the state in the local channel snapshot/status UI. This deliberately does not add upstream's automatic protocol negotiation, OAuth, or WebSocket subsystem. |

## 2026-08-30 explicitly not selected

The following items from the same upstream review are deliberately excluded and must not be reintroduced as incidental dependencies:

- `ea511025` Codex Premium passive-quota header scope replacement; this fork has no corresponding Codex OAuth passive-usage subsystem.
- `c42e6df6` and the broader management-account/check-in series; `01aa71bd` Zed changes; Cursor/xAI/Z.ai OAuth and SDK bridge series; and the large CLIProxy/Responses WebSocket synchronization series.
- `48a6a9f7`, `98825fdc`, `cece8040`, `c18a640c`, dependency/toolchain/release-only commits, and other unselected UI or catalog changes from the range after `7815e182`.
- Previously rejected commits `b8f6ffc7`, `3879fb5b`, `b435c2e3`, `629ebb49`, `bb274bb6`, `51a56a33`, and `1bce74c3` remain rejected.

This batch has been ported and verified. Use `c1a5b85e` as the next upstream content-review checkpoint.
