# 页内刷新执行报告

- Session：`20260621-execute-scratch`
- Plan：`.workflow/scratch/20260621-plan-Ppartial-refresh/plan.json`
- 完成时间：`2026-06-21T21:32:51+08:00`
- 执行方式：按 plan 的 3 个 wave 顺序执行，未启用 auto-commit。

## Wave 1 / TASK-001

实现共享基础设施：

- `web/assets/js/ui.js` 暴露 `window.CCPartialRouter`，支持 `navigate()`、`fetchPageDocument()`、`DOMParser`、`replaceMain()`、`syncPageOwnedNodes()`、`syncHeadAssets()`、`loadPageScripts()`、fallback 和 `popstate`。
- 只替换 `main.main-content`，保留 topbar/通知/background，页面级 body 附属节点同步到 `#partial-page-owned-root`。
- `initTopbar()` 变为幂等：已有 `.topbar` 时只调用 `updateTopbarActive()`，不重复 append。
- `window.CCPageLifecycle` 提供 `register/mount/unmountCurrent/onCleanup/addEvent/setAutoRefresh/disposeCharts/isMounting`。

## Wave 2 / TASK-002

实现页面接入与资源清理：

- `initPageBootstrap()` 自动注册页面 mount，局部导航加载新页面脚本时只注册不自动运行，再由 router mount。
- `withLifecycleCapture()` 捕获页面 run 期间的 `document/window.addEventListener`、`setInterval`、`window.i18n.onLocaleChange`，在离页时清理。
- `createAutoRefresh()` 在 PageLifecycle mount 中自动注册 `stop()` cleanup。
- `stats.js` 和 `logs.js` 的跨页跳转优先使用 `CCPartialRouter.navigate(targetURL)`，保留 `window.location.href = targetURL` fallback。
- `stats.js`/`trend.js` 增加图表 dispose cleanup。

## Wave 3 / TASK-003

补齐验证：

- 新增 `web/assets/js/ui-partial-router.test.js`。
- 扩展 `web/assets/js/ui-page-bootstrap.test.js`、`web/assets/js/web-refactor-guard.test.js`。
- 扩展 `internal/app/static_handler_test.go`，验证 `/web/logs.html`、`/web/stats.html` 静态整页 fallback。

## 验证命令

- `node --check web/assets/js/ui.js web/assets/js/stats.js web/assets/js/logs.js web/assets/js/trend.js`：通过。
- `node --test web/assets/js/ui-partial-router.test.js web/assets/js/ui-page-bootstrap.test.js web/assets/js/ui-delegated-actions.test.js web/assets/js/filter-state.test.js web/assets/js/web-refactor-guard.test.js`：34 pass。
- `node --test web/assets/js/ui-page-bootstrap.test.js web/assets/js/filter-state.test.js web/assets/js/stats-inline-controls.test.js web/assets/js/logs-inline-controls.test.js web/assets/js/trend-filter-state.test.js web/assets/js/tokens-inline-controls.test.js web/assets/js/model-test-inline-controls.test.js`：61 pass。
- `go test ./internal/app -run 'TestStaticFileServing|TestGetContentType|TestChannelsTemplateNameLineLayout|TestDashboardHTMLStaticFallbackSupportsPartialRouter'`：通过。
- `make www-setup && make web-test`：475 pass。
- `go test ./...`：通过。

## 注意事项

- `make web-test` 直接运行时，如果没有先复制 `www/assets/js/i18n.js`，会因该文件被 `www/.gitignore` 忽略而失败；本次按项目已有 `www-setup` 流程先复制资源后通过。
