package _115_open

import (
	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/op"
)

type Addition struct {
	// Usually one of two
	driver.RootID
	// define other
	RefreshToken   string  `json:"refresh_token" required:"true" type:"text" help:"115 refresh token"`
	OrderBy        string  `json:"order_by" type:"select" options:"name,size,update_at,create_at" default:"name" help:"order by"`
	OrderDirection string  `json:"order_direction" type:"select" options:"asc,desc" default:"asc" help:"order direction"`
	AccessToken    string  `json:"access_token" type:"text" help:"115 access token"`
	LimitRate      float64 `json:"limit_rate" type:"float" default:"2" help:"limit all api request rate ([limit]r/1s)"`
}

var config = driver.Config{
	Name:              "115Open",
	LocalSort:         false,
	OnlyLocal:         false,
	OnlyProxy:         false,
	NoCache:           false,
	NoUpload:          false,
	NeedMs:            false,
	DefaultRoot:       "0",
	CheckStatus:       false,
	Alert:             "",
	NoOverwriteUpload: false,
}

func init() {
	op.RegisterDriver(func() driver.Driver {
		return &Open115{}
	})
}
