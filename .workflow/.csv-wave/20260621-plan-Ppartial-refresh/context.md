# 原生 JS 页内刷新改造规划

- Session：`20260621-plan-Ppartial-refresh`
- Scope：standalone（仓库此前没有 `.workflow/` 状态，已为本次规划初始化）
- 目标：点击后台 tab 时不整页刷新，通过 `fetch()` 获取页面 HTML，解析并只替换 `main.main-content`，使用 `history.pushState()` 保持 URL，并支持 back/forward。
- 复杂度：medium；任务数：3；波次：3
- 冲突检测：无同里程碑计划冲突

## Wave 1 探索结果

### E1 Architecture Exploration

已完成架构探索：后端 `internal/app/server.go:SetupRoutes` 在 `/admin`、`/public` API 后挂 `setupStaticFiles`；`internal/app/static.go` 的 `serveStaticFileFrom/serveHTMLWithVersionFrom` 为 `/web/*.html` 提供 no-cache 和 `__VERSION__` 替换，局部刷新可 fetch 整页解析 `main`，新增 fragment 也须复用缓存/版本策略。前端后台页主体是 `web/*.html` 的 `.app-container > main.main-content > .content-area`；`web/assets/js/ui.js:initPageBootstrap` 统一 i18n/topbar/run，`NAVS/buildTopbar` 生成 tab，可拦截 `.topnav-link[data-nav-key]`。可复用 `fetchWithAuth/fetchDataWithAuth`、`initDelegatedActions`、`FilterState.writeHistory/persistFilterState`、`PageFilters.initPageFilters`。风险：页面脚本为全局 classic JS，重复执行/未卸载会变量冲突、重复监听；channels/tokens/logs/model-test 的 modal/template 多在 main 外，单替换 main 会丢节点。

### E2 Implementation Exploration

找到可复用模式：1）`web/assets/js/ui.js:initPageBootstrap/initDelegatedActions/createAutoRefresh`，统一页面启动、导航高亮、事件委托与刷新，可作为局部替换后的 mount 入口；2）`web/assets/js/logs-channel-editor.js:getVersionedAssetURL/fetchChannelsDocument/loadScriptOnce`，已有 fetch HTML、DOMParser、按版本去重加载脚本模式；3）`web/assets/js/filter-state.js:writeHistory` 与 `ui.js:persistFilterState` 已封装 pushState/replaceState；4）`web/*.html` 都有 `<main class="main-content">` 和按页脚本清单；5）`internal/app/static.go` HTML no-cache + `__VERSION__` 可直接 fetch 整页。风险：脚本非完全可重入，需 cleanup/去重。已追加 discoveries。

### E3 Integration Exploration

触点：web/assets/js/ui.js 的 NAVS/buildTopbar 是后台 tab 源头，需拦截 .topnav-link 并 fetch/替换 main.main-content、pushState、同步 active；initPageBootstrap 已统一页面初始化，可扩展为 mount/unmount。FilterState/writeHistory 与 persistFilterState 已服务 logs/stats/trend 的 query/replaceState。跨页入口在 stats.js 跳 logs、logs.js 跳 channels?id#hash。static.go 仅服务整页 HTML/静态资源；可 fetch 整页抽 main，若局部端点需补 static_handler_test。风险：页面脚本重复绑定 interval/resize/hashchange/pageshow。

### E4 Risk Exploration

风险触点：1）`ui.js:initPageBootstrap/initTopbar` 非幂等，局部重跑会重复 topbar；应拆 app shell 与 `updateTopbarActive`。2）页面脚本（`channels-init/logs/stats/trend/tokens/model-test`）有全局 `let/const`、`keydown/pageshow/hashchange/resize`、`setInterval/createAutoRefresh`、ECharts，需 PageLifecycle mount/unmount，清理监听、timer、chart.dispose。3）`FilterState.writeHistory` 仅写 URL，无 `popstate` 恢复；路由层需按 URL 重挂载并保留 query/hash。4）`web/*.html` 的 modal/template 多在 main 外，main-only 替换会缺失；需定义 fragment 边界。5）`static.go` 可 fetch 整页 HTML 且 HTML no-cache，保留静态 fallback；新增测试覆盖 partial/popstate/auth 401。已向发现板追加风险与测试命令。

## 计划概览

### Wave 1：TASK-001

建立共享局部路由、应用壳幂等初始化、HTML 解析/资产同步与 history/popstate 基础能力。

### Wave 2：TASK-002

让各后台页面脚本以 PageLifecycle 挂载/卸载方式运行，处理复杂页面资源清理与跨页跳转入口。

### Wave 3：TASK-003

补齐聚焦回归测试和静态 fallback 测试，验证不整页刷新、back/forward、只替换 main 与资源去重。

### TASK-001 共享局部路由与应用壳生命周期基础

- wave：1
- depends_on：无
- files：
  - `web/assets/js/ui.js`：新增 CCPartialRouter、PageLifecycle、fetch+DOMParser、main 替换、topbar active 更新、history.pushState/popstate 与 fallback；同时让 initTopbar/initPageBootstrap 幂等。
  - `web/assets/js/logs-channel-editor.js`：参考并可抽取/复用 getVersionedAssetURL、DOMParser fetch、loadScriptOnce 去重加载模式；必要时保留兼容导出。
  - `web/assets/js/ui-page-bootstrap.test.js`：扩展共享 bootstrap/topbar 幂等与 PageLifecycle 注册行为测试。
  - `web/assets/js/ui-partial-router.test.js`：新增局部路由单元测试，覆盖 HTML 解析、main/附属节点同步、history 与 popstate。
- 关键验收：
  - web/assets/js/ui.js 中可 grep 到 `window.CCPartialRouter`、`navigate(`、`popstate`、`DOMParser`、`main.main-content`。
  - web/assets/js/ui.js 中可 grep 到 `updateTopbarActive`，且 initTopbar 不再无条件 append 新 `.topbar`。
  - web/assets/js/ui.js 中可 grep 到 `window.CCPageLifecycle` 或等价 PageLifecycle 全局对象，且包含 cleanup/onCleanup 语义。
  - 新增/更新测试覆盖：topbar 重入不产生多个 `.topbar`、tab click 调用 pushState、popstate 不 pushState、fetch HTML 缺 main 时 fallback。

### TASK-002 页面脚本接入 PageLifecycle、清理资源并改造跨页跳转

- wave：2
- depends_on：TASK-001
- files：
  - `web/assets/js/index.js`：接入 PageLifecycle mount/cleanup，保存 createAutoRefresh 句柄。
  - `web/assets/js/channels-init.js`：接入 PageLifecycle；清理 hashchange/pageshow/keydown/i18n/auto-refresh，并支持 URL hash 定位。
  - `web/assets/js/logs.js`：接入 PageLifecycle；清理 document click/keydown/pageshow/debug polling；将 channels 跳转改用 CCPartialRouter.navigate。
  - `web/assets/js/stats.js`：接入 PageLifecycle；清理 auto-refresh、resize/themechange、ECharts；将 stats→logs 跳转改用 CCPartialRouter.navigate。
  - `web/assets/js/trend.js`：接入 PageLifecycle；清理 resize/themechange/interval/channel filter 外部点击和 ECharts。
  - `web/assets/js/tokens.js`：接入 PageLifecycle；清理 keydown/i18n/auto-refresh 和容器事件重复绑定。
  - `web/assets/js/model-test.js`：接入 PageLifecycle；清理 document keydown/click、modal 临时监听与 logs-channel-editor 相关附属节点依赖。
  - `web/assets/js/settings.js`：接入 PageLifecycle；保持动态表格事件在新 main 上重绑且不重复。
  - `web/*.html`：必要时为 body 级 modal/template 添加 data-partial-owned/data-page-key 边界标记；保持 main.main-content 结构不变。
- 关键验收：
  - web/assets/js/stats.js 和 web/assets/js/logs.js 中可 grep 到 `CCPartialRouter.navigate`，且仍保留 `window.location.href` fallback。
  - web/assets/js/{index.js,channels-init.js,logs.js,stats.js,trend.js,tokens.js,model-test.js,settings.js} 中可 grep 到 `initPageBootstrap({` 或 `CCPageLifecycle.register`，每页保留对应 topbarKey。
  - stats.js/trend.js 中可 grep 到 `dispose(` 或等价 chart cleanup；使用 createAutoRefresh 的页面可 grep 到 `.stop()` cleanup。
  - logs.js 中 activeDebugLogRefreshTimer 在离页 cleanup 中停止；channels-init.js 中 hashchange/pageshow/keydown 监听通过 cleanup 移除或全局 once guard。
  - web/*.html 仍全部包含 `<main class="main-content`，且需要的 modal/template 具备 data-partial-owned 标记或被 TASK-001 测试证明可自动同步。

### TASK-003 局部刷新回归测试与静态 fallback 验证

- wave：3
- depends_on：TASK-001, TASK-002
- files：
  - `web/assets/js/ui-partial-router.test.js`：新增/完善 router 单元测试，覆盖 tab 拦截、fetch/DOMParser、main-only 替换、owned fragments、history、popstate、fallback。
  - `web/assets/js/ui-page-bootstrap.test.js`：强化 bootstrap/PageLifecycle 约束，防止页面重新直接绑定 DOMContentLoaded 或重复 topbar。
  - `web/assets/js/filter-state.test.js`：确认 FilterState 仍只负责筛选 URL 写回，router popstate 不破坏 query 恢复。
  - `web/assets/js/web-refactor-guard.test.js`：可加入源码级 guard：禁止新增直接 history.pushState/replaceState 和无 fallback 的 location.href。
  - `internal/app/static_handler_test.go`：补充 /web/*.html 静态 fallback、no-cache、__VERSION__、main 结构存在性测试；若未新增后端 fragment，显式保护现有整页 HTML 策略。
  - `internal/app/static.go`：只作为测试依据读取；默认不改，除非执行中确需新增 fragment endpoint。
- 关键验收：
  - `node --test web/assets/js/ui-partial-router.test.js` 通过，且测试名包含 partial navigation、popstate、fallback/main missing。
  - `node --test web/assets/js/ui-page-bootstrap.test.js web/assets/js/filter-state.test.js web/assets/js/web-refactor-guard.test.js` 通过。
  - `go test ./internal/app -run 'TestStaticFileServing|TestGetContentType|TestChannelsTemplateNameLineLayout'` 通过；若新增 HTML fallback 子测试，同一命令覆盖。
  - `make web-test` 通过或仅有与本改造无关的既有失败且需记录失败测试名与原因。

## 建议下一步

- 执行计划：读取 `.workflow/scratch/20260621-plan-Ppartial-refresh/plan.json` 和 `.task/TASK-*.json`，按 Wave 1 → 2 → 3 顺序实施。
- 聚焦验证优先：`node --test web/assets/js/ui-page-bootstrap.test.js web/assets/js/ui-delegated-actions.test.js web/assets/js/filter-state.test.js web/assets/js/ui-partial-router.test.js`。
- 完整前端回归：`make web-test`。
