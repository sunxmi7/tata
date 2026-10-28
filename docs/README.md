# HTTP API 文档

机器可读契约：[openapi.yaml](./openapi.yaml)（OpenAPI 3.0.3）。

路径、方法、请求体和响应形状来自 `server/internal/tata/routes.go` 及对应 handler，只收录已注册的接口。

2026-10-28 起 App P0 壳层路径包括：`GET/PUT /api/app/v1/profile`（name/bio/avatar_url、account_display）、`GET /api/app/v1/explore/topics`、`POST /api/app/v1/redeem`、`GET /api/app/v1/wallet/ledger`、`POST /api/app/v1/uploads`、`POST /api/app/v1/wishlist`、`POST /api/app/v1/feedback`、`GET/PUT /api/app/v1/notification-preferences`。钻石账本的 UI 展示名为「塔塔币」（`display_label`）。

同日 App P1：`GET /api/app/v1/redeem/records`、`GET /api/app/v1/campaigns`、`GET /api/app/v1/campaigns/{id}`、`GET /api/app/v1/campaigns/participations`、`GET /api/app/v1/campaigns/rewards`、`GET /api/app/v1/help/categories`、`GET /api/app/v1/help/articles`（`category`、`q`）、`GET /api/app/v1/help/articles/{id}`。兑换记录、活动详情、参与记录、奖励记录和 FAQ 正文仍是占位：列表可为空，不编造记录；FAQ `body` 为「待确认」。`POST /api/app/v1/push/register` 未注册：通知设置没有推送 token 字段。

## 导入 Apifox

在 Apifox 中选择 **导入 → OpenAPI**，选择本仓库的 `docs/openapi.yaml`。

导入后把环境的服务地址设为 `http://127.0.0.1:8080`（`server/manifest/config/config.yaml` 里 `server.address` 的默认值）。App 接口使用安全方案 **AppBearer**（JWT `aud=app`），管理端接口使用 **AdminBearer**（JWT `aud=admin`）。两种 access token 不能混用。

本地种子账号只用于本机演示库：`admin` / `admin123`（可写），`readonly` / `readonly123`（只读）。只读角色调用写接口会得到 `code=1`、`message=只读账号不能修改`。
