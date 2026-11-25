# App 月报 / 年报 Mock

结构对齐 `openapi-app.yaml` 的 `MonthlyReport` / `YearlyReport` / `YearlyShare`。

## 推荐：直接打接口

服务端已支持演示数据（需登录）：

| 调用 | 结果 |
|---|---|
| `GET /api/app/v1/monthly-reports` | 有数据月报（`reports.demo=true` 或 `?demo=1`） |
| `GET /api/app/v1/monthly-reports?demo=empty` | 空月报 |
| `GET /api/app/v1/yearly-reports/2026` | 完整年运（同上） |
| `GET /api/app/v1/yearly-reports/2026?demo=locked` | 未解锁卖点页 |
| `GET /api/app/v1/yearly-reports/2026/share?demo=1` | 分享图 |

本地 `server/manifest/config/config.yaml` 已开 `reports.demo: true`。打包到测试环境时，在 `tata-deploy` 的同文件加上这段，App **不用改参数**就能拿到展示数据。正式环境务必关掉。

## 静态 JSON（备用）

| 文件 | 场景 |
|---|---|
| `monthly-report-ready.json` | 有数据月报 |
| `monthly-report-empty.json` | 空月报 |
| `yearly-report-locked.json` | 未解锁 |
| `yearly-report-ready.json` | 完成态 |
| `yearly-share.json` | 分享图 |
