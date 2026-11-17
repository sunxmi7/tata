// 部署后改这一处即可，不用重新打包。
// 同源 Nginx 把 /api 反代到 Go：留 "/api/admin/v1"
// 接口在别的域名：写成完整地址
window.__TATA_ADMIN_API_URL__ = 'https://dev.tarot.himoou.com/api/admin/v1';
