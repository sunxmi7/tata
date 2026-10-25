package tata

import (
	"context"
	"time"

	_ "github.com/gogf/gf/contrib/nosql/redis/v2"
	"github.com/gogf/gf/v2/frame/g"
)

type gfRedis struct{}

func RedisFromFrame() redisLocker {
	return gfRedis{}
}

func NewRedisLock(addr string) (redisLocker, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, err := g.Redis().Do(ctx, "PING"); err != nil {
		return nil, err
	}
	return gfRedis{}, nil
}

func (gfRedis) Lock(key string, ttl time.Duration) (bool, error) {
	ctx := context.Background()
	v, err := g.Redis().Do(ctx, "SET", key, "1", "NX", "EX", int(ttl.Seconds()))
	if err != nil || v == nil || v.IsNil() {
		return false, err
	}
	return v.String() == "OK", nil
}

func (gfRedis) Unlock(key string) error {
	_, err := g.Redis().Do(context.Background(), "DEL", key)
	return err
}
