# TASK-002 执行摘要

## 状态

completed

## 实现

- `initPageBootstrap()` 注册页面 mount，局部导航时避免自动执行脚本，由 router mount 触发。
- `withLifecycleCapture()` 捕获页面 run 期间注册的 window/document 事件、timer 和 i18n 订阅，离页时 cleanup。
- `createAutoRefresh()` 在生命周期 mount 中自动注册 stop cleanup。
- `stats.js`、`logs.js` 跨页跳转改为优先 `CCPartialRouter.navigate(targetURL)`，保留 `window.location.href` fallback。
- `stats.js`、`trend.js` 注册 chart dispose cleanup。

## 文件

- `web/assets/js/ui.js`
- `web/assets/js/stats.js`
- `web/assets/js/logs.js`
- `web/assets/js/trend.js`

## 验证

- `node --check web/assets/js/ui.js web/assets/js/stats.js web/assets/js/logs.js web/assets/js/trend.js`
- 页面聚焦测试：61 pass
- `make www-setup && make web-test`：475 pass
