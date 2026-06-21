# TASK-003 执行摘要

## 状态

completed

## 实现

- 新增 `ui-partial-router.test.js`，覆盖 router API、DOMParser/main 替换入口、popstate 不 push、fallback/main missing、PageLifecycle cleanup。
- 扩展 bootstrap 和 refactor guard 测试。
- 扩展 Go 静态服务测试，确认 HTML fallback 返回 no-cache、版本替换后页面仍包含 `main.main-content`。

## 文件

- `web/assets/js/ui-partial-router.test.js`
- `web/assets/js/ui-page-bootstrap.test.js`
- `web/assets/js/web-refactor-guard.test.js`
- `internal/app/static_handler_test.go`

## 验证

- `node --test ...` 聚焦测试通过。
- `make www-setup && make web-test`：475 pass。
- `go test ./...`：通过。
