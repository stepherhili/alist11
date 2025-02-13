package _123LinkDir

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/alist-org/alist/v3/drivers/base"
	"github.com/alist-org/alist/v3/internal/model"
)

// do others that not defined in Driver interface
func (d *Pan123LinkDir) GetFileStat(fileID string) (*File, error) {
	url := OpenAPIBaseURL + "/api/v1/file/detail"

	req := base.RestyClient.R().
		SetQueryParam("fileId", fileID).
		SetHeader("Authorization", "Bearer "+d.access_token).
		SetHeader("Platform", "open_platform")

	res, err := req.Execute(http.MethodGet, url)
	if err != nil {
		return nil, err
	}

	resStruct := struct {
		Data File `json:"data"`
	}{}

	fmt.Println(string(res.Body()))

	err = json.Unmarshal(res.Body(), &resStruct)
	if err != nil {
		return nil, err
	}

	return &resStruct.Data, nil
}

func GetObjID(obj model.Obj) string {
	objID := obj.GetID()
	if objID == "" {
		objID = "0"
	}
	return objID
}
