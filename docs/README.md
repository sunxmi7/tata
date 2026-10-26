# HTTP API 文档

机器可读契约：[openapi.yaml](./openapi.yaml)（OpenAPI 3.0.3）。

路径、方法、请求体和响应形状来自 `server/internal/tata/routes.go` 及对应 handler，只收录已注册的接口。

## 导入 Apifox

在 Apifox 中选择 **导入 → OpenAPI**，选择本仓库的 `docs/openapi.yaml`。

导入后把环境的服务地址设为 `http://127.0.0.1:8080`（`server/manifest/config/config.yaml` 里 `server.address` 的默认值）。App 接口使用安全方案 **AppBearer**（JWT `aud=app`），管理端接口使用 **AdminBearer**（JWT `aud=admin`）。两种 access token 不能混用。

本地种子账号只用于本机演示库：`admin` / `admin123`（可写），`readonly` / `readonly123`（只读）。只读角色调用写接口会得到 `code=1`、`message=只读账号不能修改`。
