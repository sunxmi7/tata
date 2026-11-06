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
			g.Log().Warningf(ctx, "redis unavailable, generation lock falls back to database: %v", err)
		} else {
			app.Redis = tata.RedisFromFrame()
			g.Log().Info(ctx, "redis lock enabled")
		}
		s := g.Server()
		app.Bind(s)
		g.Log().Info(ctx, "admin http://127.0.0.1:8080/admin  vben http://127.0.0.1:5666/")
		s.Run()
		return nil
	},
}
