package _123LinkDir

import (
	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/op"
)

const (
	OpenAPIBaseURL = "https://open-api.123pan.com"
)

type Addition struct {
	// 通常是两个之一
	driver.RootPath
	driver.RootID

	// 自定义域名 例如 vip.123pan.cn
	Domain string `json:"domain" type:"text" required:"true" help:"The domain used for accessing the service" default:"vip.123pan.cn"`
	// 是否启用HTTPS
	EnableHTTPS bool `json:"enable_https" type:"bool" default:"false"`
	// 下载链接中的UID 不设置为隐藏
	UUID string `json:"uuid" type:"text"`
	// 下载时的直链鉴权密钥
	DownloadKey string `json:"download_key" type:"text"`

	// OpenAPI 相关
	ClientID     string `json:"client_id" type:"text"`
	ClientSecret string `json:"client_secret" type:"text"`

	RootFolderID int `json:"root_folder_id" type:"number"`
	access_token string
}

var config = driver.Config{
	Name: "123PanLinkDir",
}

func init() {
	op.RegisterDriver(func() driver.Driver {
		return &Pan123LinkDir{}
	})
}
