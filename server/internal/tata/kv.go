package tata

import (
	"strconv"
	"sync"
	"time"
)

type kvStore interface {
	Set(key, val string, ttl time.Duration) error
	SetNX(key, val string, ttl time.Duration) (bool, error)
	Get(key string) (string, bool, error)
	Del(key string) error
	IncrExpire(key string, ttl time.Duration) (int64, error)
}

type memItem struct {
	val string
	exp time.Time
}

type memoryKV struct {
	mu   sync.Mutex
	data map[string]memItem
}

func newMemoryKV() *memoryKV {
	return &memoryKV{data: map[string]memItem{}}
}

func MemoryKV() kvStore { return newMemoryKV() }

func (m *memoryKV) Set(key, val string, ttl time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[key] = memItem{val: val, exp: time.Now().Add(ttl)}
	return nil
}

func (m *memoryKV) SetNX(key, val string, ttl time.Duration) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if item, ok := m.data[key]; ok && time.Now().Before(item.exp) {
		return false, nil
	}
	m.data[key] = memItem{val: val, exp: time.Now().Add(ttl)}
	return true, nil
}

func (m *memoryKV) Get(key string) (string, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	item, ok := m.data[key]
	if !ok || time.Now().After(item.exp) {
		delete(m.data, key)
		return "", false, nil
	}
	return item.val, true, nil
}

func (m *memoryKV) Del(key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, key)
	return nil
}

func (m *memoryKV) IncrExpire(key string, ttl time.Duration) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	item, ok := m.data[key]
	if !ok || now.After(item.exp) {
		m.data[key] = memItem{val: "1", exp: now.Add(ttl)}
		return 1, nil
	}
	n, _ := strconv.ParseInt(item.val, 10, 64)
	n++
	item.val = strconv.FormatInt(n, 10)
	m.data[key] = item
	return n, nil
}
