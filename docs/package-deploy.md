# 打包到 tata-deploy

把本仓库编好的后台、管理端和 App 接口文档放到旁边的 `tata-deploy`。线上用那份目录发布。本地 `:8080` 不要动。

默认目标：`/Users/xiaomengsun/Developer/tata-deploy`

## 一条命令

在仓库根目录：

```bash
chmod +x scripts/package-deploy.sh
./scripts/package-deploy.sh
```

只要其中一块：

```bash
./scripts/package-deploy.sh --server
./scripts/package-deploy.sh --admin
./scripts/package-deploy.sh --docs
```

目标目录不在默认位置时：

```bash
./scripts/package-deploy.sh --deploy /path/to/tata-deploy
# 或
TATA_DEPLOY=/path/to/tata-deploy ./scripts/package-deploy.sh
```

脚本会拒绝在还不存在的目标目录里新建一份空部署，避免漏掉线上 `config.yaml`。

## 会生成什么

| 产物 | 来源 | 放到 |
|---|---|---|
| Linux amd64 静态二进制 | `server/cmd/tata` | `tata-deploy/server/tata-linux-amd64` |
| 管理端静态资源 | `admin/` 里 `pnpm run build:antd` | `tata-deploy/admin/`（整目录覆盖，旧 hash 文件会删） |
| App 接口文档 | `docs/openapi-app.yaml` | `tata-deploy/docs/openapi-app.yaml` |

管理端生产环境已经写在 `admin/apps/web-antd/.env.production`：

- `VITE_BASE=/`
- `VITE_GLOB_API_URL=/api/admin/v1`
- `VITE_ROUTER_HISTORY=hash`

编出来的 `index.html` 必须在 `tata-deploy/admin/index.html`。

## 不要动这些

- `tata-deploy/server/manifest/config/config.yaml`（线上 SMTP、JWT、数据库）
- `tata-deploy/server/uploads/`（牌图、已上传文件，尤其 `uploads/decks/`）
- 本机正在跑的 `:8080`

App 和后台是同一个 Go 二进制。管理端是静态文件，由线上 Nginx 或同机静态目录提供。

## 手工命令

不想跑脚本时，等价步骤如下。

后台（在 `server/`）：

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GOPROXY=https://proxy.golang.org,direct \
  go build -trimpath -ldflags "-s -w" \
  -o /tmp/tata-linux-amd64 ./cmd/tata
cp /tmp/tata-linux-amd64 /Users/xiaomengsun/Developer/tata-deploy/server/tata-linux-amd64
file /Users/xiaomengsun/Developer/tata-deploy/server/tata-linux-amd64
```

应看到 `ELF 64-bit LSB executable, x86-64`，静态链接，大约 40MB。

管理端（在 `admin/`）：

```bash
pnpm run build:antd
rsync -a --delete --exclude '*.zip' \
  apps/web-antd/dist/ \
  /Users/xiaomengsun/Developer/tata-deploy/admin/
```

接口文档：

```bash
cp docs/openapi-app.yaml /Users/xiaomengsun/Developer/tata-deploy/docs/openapi-app.yaml
```

Apifox 导入用仓库里的 `docs/openapi-app.yaml`。deploy 那份给线上对照，不要导入 `openapi.yaml`。

## 编完之后

1. 看脚本打印的 `file` 结果，确认是 Linux amd64。
2. 打开 `tata-deploy/admin/index.html`，确认有这份文件。
3. 把 `tata-deploy` 同步到服务器。
4. 在服务器上自己替换二进制并重启进程。本机不要重启。
