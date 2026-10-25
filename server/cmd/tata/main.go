package main

import (
	_ "github.com/gogf/gf/contrib/drivers/pgsql/v2"
	_ "github.com/gogf/gf/contrib/nosql/redis/v2"

	"github.com/gogf/gf/v2/os/gctx"

	"tata/server/internal/cmd"
)

func main() {
	cmd.Main.Run(gctx.GetInitCtx())
}
