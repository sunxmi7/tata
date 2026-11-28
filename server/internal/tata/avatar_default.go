package tata

import (
	_ "embed"
	"net/http"
	"strings"
)

const (
	defaultAvatarName = "avatar-default.png"
	defaultAvatarPath = "/uploads/avatar-default.png"
)

//go:embed assets/avatar-default.png
var defaultAvatarPNG []byte

func profileAvatarURL(r *http.Request, stored string) any {
	stored = strings.TrimSpace(stored)
	if stored == "" || stored == defaultAvatarPath {
		stored = defaultAvatarPath
	}
	return emptyAsNil(absoluteAssetURL(r, stored))
}
