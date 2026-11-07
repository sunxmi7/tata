package tata

import (
	"bytes"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/aliyun/aliyun-oss-go-sdk/oss"
)

// UploadConfig 对应 config.yaml 的 upload。driver=local 写本机；driver=oss 传到阿里云 OSS。
type UploadConfig struct {
	Driver   string
	LocalDir string
	OSS      OSSConfig
}

type OSSConfig struct {
	Endpoint        string
	Bucket          string
	AccessKeyId     string
	AccessKeySecret string
	Prefix          string
	PublicBaseURL   string
}

// Uploader 按配置把图片落到本地或 OSS。putOSS 仅测试注入。
type Uploader struct {
	cfg    UploadConfig
	putOSS func(key, contentType string, data []byte) error
}

func NewUploader(cfg UploadConfig) *Uploader {
	return &Uploader{cfg: cfg}
}

func (u *Uploader) Driver() string {
	if u == nil {
		return "local"
	}
	d := strings.ToLower(strings.TrimSpace(u.cfg.Driver))
	if d == "" {
		return "local"
	}
	return d
}

func (u *Uploader) OSSReady() bool {
	if u == nil {
		return false
	}
	o := u.cfg.OSS
	return strings.TrimSpace(o.Endpoint) != "" &&
		strings.TrimSpace(o.Bucket) != "" &&
		strings.TrimSpace(o.AccessKeyId) != "" &&
		strings.TrimSpace(o.AccessKeySecret) != ""
}

func (u *Uploader) localDir() string {
	if u != nil {
		if dir := strings.TrimSpace(u.cfg.LocalDir); dir != "" {
			return dir
		}
	}
	return filepath.Join(os.TempDir(), "tata-uploads")
}

func (u *Uploader) Save(r *http.Request, stored string, data []byte, contentType string) (string, error) {
	switch u.Driver() {
	case "local":
		return u.saveLocal(r, stored, data)
	case "oss":
		return u.saveOSS(stored, data, contentType)
	default:
		return "", biz("上传方式不正确")
	}
}

func (u *Uploader) saveLocal(r *http.Request, stored string, data []byte) (string, error) {
	path := filepath.Join(u.localDir(), filepath.FromSlash(stored))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", err
	}
	return publicAssetURL(r, stored), nil
}

func (u *Uploader) saveOSS(stored string, data []byte, contentType string) (string, error) {
	if u.putOSS == nil && !u.OSSReady() {
		return "", biz("OSS 未配置")
	}
	key := ossObjectKey(u.cfg.OSS.Prefix, stored)
	ct := strings.TrimSpace(contentType)
	if ct == "" {
		ct = "application/octet-stream"
	}
	put := u.putOSS
	if put == nil {
		put = u.putOSSReal
	}
	if err := put(key, ct, data); err != nil {
		log.Printf("oss put: %v", err)
		return "", biz("上传到 OSS 失败")
	}
	return ossPublicURL(u.cfg.OSS, key), nil
}

func (u *Uploader) putOSSReal(key, contentType string, data []byte) error {
	client, err := oss.New(strings.TrimSpace(u.cfg.OSS.Endpoint), strings.TrimSpace(u.cfg.OSS.AccessKeyId), strings.TrimSpace(u.cfg.OSS.AccessKeySecret))
	if err != nil {
		return err
	}
	bucket, err := client.Bucket(strings.TrimSpace(u.cfg.OSS.Bucket))
	if err != nil {
		return err
	}
	return bucket.PutObject(key, bytes.NewReader(data), oss.ContentType(contentType))
}

func ossObjectKey(prefix, stored string) string {
	p := strings.Trim(strings.ReplaceAll(strings.TrimSpace(prefix), "\\", "/"), "/")
	if p == "" {
		return stored
	}
	return p + "/" + stored
}

func ossPublicURL(cfg OSSConfig, key string) string {
	base := strings.TrimRight(strings.TrimSpace(cfg.PublicBaseURL), "/")
	if base != "" {
		return base + "/" + key
	}
	endpoint := strings.TrimSpace(cfg.Endpoint)
	endpoint = strings.TrimPrefix(endpoint, "https://")
	endpoint = strings.TrimPrefix(endpoint, "http://")
	endpoint = strings.TrimRight(endpoint, "/")
	bucket := strings.TrimSpace(cfg.Bucket)
	if bucket == "" || endpoint == "" {
		return "/" + key
	}
	return "https://" + bucket + "." + endpoint + "/" + key
}
