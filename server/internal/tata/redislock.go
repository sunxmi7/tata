package tata

import (
	"context"
	"time"

	_ "github.com/gogf/gf/contrib/nosql/redis/v2"
	"github.com/gogf/gf/v2/frame/g"
)

type gfRedis struct{}

func RedisFromFrame() gfRedis {
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

func (gfRedis) Set(key, val string, ttl time.Duration) error {
	_, err := g.Redis().Do(context.Background(), "SET", key, val, "EX", int(ttl.Seconds()))
	return err
}

func (gfRedis) SetNX(key, val string, ttl time.Duration) (bool, error) {
	v, err := g.Redis().Do(context.Background(), "SET", key, val, "NX", "EX", int(ttl.Seconds()))
	if err != nil || v == nil || v.IsNil() {
		return false, err
	}
	return v.String() == "OK", nil
}

func (gfRedis) Get(key string) (string, bool, error) {
	v, err := g.Redis().Do(context.Background(), "GET", key)
	if err != nil {
		return "", false, err
	}
	if v == nil || v.IsNil() {
		return "", false, nil
	}
	return v.String(), true, nil
}

func (gfRedis) Del(key string) error {
	_, err := g.Redis().Do(context.Background(), "DEL", key)
	return err
}

func (gfRedis) IncrExpire(key string, ttl time.Duration) (int64, error) {
	ctx := context.Background()
	v, err := g.Redis().Do(ctx, "INCR", key)
	if err != nil {
		return 0, err
	}
	n := v.Int64()
	if n == 1 {
		_, err = g.Redis().Do(ctx, "EXPIRE", key, int(ttl.Seconds()))
	}
	return n, err
}
