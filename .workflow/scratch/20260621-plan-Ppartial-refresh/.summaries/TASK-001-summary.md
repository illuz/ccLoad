# TASK-001 执行摘要

## 状态

completed

## 实现

- 在 `web/assets/js/ui.js` 新增 `CCPartialRouter`：fetch 整页 HTML、`DOMParser` 解析、抽取并替换 `main.main-content`、同步 title/body/owned nodes/head assets、串行加载页面脚本、成功后更新 history。
- 新增 `CCPageLifecycle`：页面注册、挂载、卸载和 cleanup 管理。
- `initTopbar()` 改为幂等，已有 `.topbar` 时只更新 active/auth 状态。

## 文件

- `web/assets/js/ui.js`
- `web/assets/js/ui-page-bootstrap.test.js`
- `web/assets/js/ui-partial-router.test.js`
- `web/assets/js/web-refactor-guard.test.js`

## 验证

- `node --check web/assets/js/ui.js`
- 聚焦 router/bootstrap/filter/guard 测试：34 pass
