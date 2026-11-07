package cmd

import (
	"context"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gcmd"

	"tata/server/internal/tata"
)

var Main = gcmd.Command{
	Name:  "tata",
	Usage: "tata",
	Brief: "塔塔后端，GoFrame + Eino",
	Func: func(ctx context.Context, parser *gcmd.Parser) (err error) {
		store, err := tata.Open(ctx, "", []byte("0123456789abcdef0123456789abcdef"))
		if err != nil {
			return err
		}
		app := &tata.App{Store: store, JWTSecret: []byte("tata-demo-jwt-secret-change-me")}
		app.Upload = tata.NewUploader(tata.UploadConfig{
			Driver:   g.Cfg().MustGet(ctx, "upload.driver").String(),
			LocalDir: g.Cfg().MustGet(ctx, "upload.localDir").String(),
			OSS: tata.OSSConfig{
				Endpoint:        g.Cfg().MustGet(ctx, "upload.oss.endpoint").String(),
				Bucket:          g.Cfg().MustGet(ctx, "upload.oss.bucket").String(),
				AccessKeyId:     g.Cfg().MustGet(ctx, "upload.oss.accessKeyId").String(),
				AccessKeySecret: g.Cfg().MustGet(ctx, "upload.oss.accessKeySecret").String(),
				Prefix:          g.Cfg().MustGet(ctx, "upload.oss.prefix").String(),
				PublicBaseURL:   g.Cfg().MustGet(ctx, "upload.oss.publicBaseUrl").String(),
			},
		})
		switch app.Upload.Driver() {
		case "oss":
			if app.Upload.OSSReady() {
				g.Log().Info(ctx, "upload driver oss")
			} else {
				g.Log().Warning(ctx, "upload driver oss but unconfigured")
			}
		default:
			g.Log().Info(ctx, "upload driver local")
		}
		app.Mail = tata.NewSMTPMailer(tata.MailConfig{
			Enabled:  g.Cfg().MustGet(ctx, "mail.enabled").Bool(),
			Host:     g.Cfg().MustGet(ctx, "mail.host").String(),
			Port:     g.Cfg().MustGet(ctx, "mail.port").Int(),
			Username: g.Cfg().MustGet(ctx, "mail.username").String(),
			Password: g.Cfg().MustGet(ctx, "mail.password").String(),
			From:     g.Cfg().MustGet(ctx, "mail.from").String(),
			FromName: g.Cfg().MustGet(ctx, "mail.fromName").String(),
		})
		if app.Mail.Ready() {
			g.Log().Info(ctx, "mail smtp enabled")
		} else {
			g.Log().Info(ctx, "mail smtp disabled (unconfigured); email codes will not send")
		}
		app.Push = tata.NewPusher(tata.PushConfig{
			AppID:        g.Cfg().MustGet(ctx, "getui.appId").String(),
			AppKey:       g.Cfg().MustGet(ctx, "getui.appKey").String(),
			AppSecret:    g.Cfg().MustGet(ctx, "getui.appSecret").String(),
			MasterSecret: g.Cfg().MustGet(ctx, "getui.masterSecret").String(),
			BaseURL:      g.Cfg().MustGet(ctx, "getui.baseUrl").String(),
			Enabled:      g.Cfg().MustGet(ctx, "getui.enabled").Bool(),
		})
		if app.Push.Ready() {
			g.Log().Info(ctx, "getui push enabled")
		} else {
			g.Log().Info(ctx, "getui push disabled (unconfigured); only in-app messages stored")
		}
		if _, err = g.Redis().Do(ctx, "PING"); err != nil {
			app.KV = tata.MemoryKV()
			g.Log().Warningf(ctx, "redis unavailable, generation lock and email codes stay in-process: %v", err)
		} else {
			r := tata.RedisFromFrame()
			app.Redis = r
			app.KV = r
			g.Log().Info(ctx, "redis lock enabled")
		}
		s := g.Server()
		app.Bind(s)
		g.Log().Info(ctx, "admin http://127.0.0.1:8080/admin  vben http://127.0.0.1:5666/")
		s.Run()
		return nil
	},
}
