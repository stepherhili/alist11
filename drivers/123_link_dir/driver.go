package _123LinkDir

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/alist-org/alist/v3/drivers/base"
	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/errs"
	"github.com/alist-org/alist/v3/internal/model"
)

const DIRVER_API = "https://open-api.123pan.com"

type Pan123LinkDir struct {
	model.Storage
	Addition
}

func (d *Pan123LinkDir) Config() driver.Config {
	return config
}

func (d *Pan123LinkDir) GetAddition() driver.Additional {
	return &d.Addition
}

func (d *Pan123LinkDir) Init(ctx context.Context) error {
	req := base.RestyClient.R()
	req.SetHeader(
		"Platform", "open_platform",
	)
	req.SetFormData(map[string]string{
		"client_id":     d.ClientID,
		"client_secret": d.ClientSecret,
	})

	res, err := req.Execute(http.MethodPost, OpenAPIBaseURL+"/api/v1/access_token")
	if err != nil {
		return err
	}

	body := res.Body()

	resStruct := struct {
		Data struct {
			AccessToken string `json:"accessToken"`
		} `json:"data"`
	}{}

	err = json.Unmarshal(body, &resStruct)
	if err != nil {
		return err
	}

	d.access_token = resStruct.Data.AccessToken

	return nil
}

func (d *Pan123LinkDir) Drop(ctx context.Context) error {
	return nil
}

func (d *Pan123LinkDir) List(ctx context.Context, dir model.Obj, args model.ListArgs) ([]model.Obj, error) {
	url := OpenAPIBaseURL + "/api/v2/file/list"

	req := base.RestyClient.R().
		SetQueryParam("parentFileId", GetObjID(dir)).
		SetQueryParam("limit", "100").
		SetHeader("Authorization", "Bearer "+d.access_token).
		SetHeader("Platform", "open_platform")

	res, err := req.Execute(http.MethodGet, url)
	if err != nil {
		return nil, err
	}

	body := res.Body()
	bodyStruct := struct {
		Data struct {
			FileList []File `json:"fileList"`
		} `json:"data"`
	}{}

	err = json.Unmarshal(body, &bodyStruct)
	if err != nil {
		return nil, err
	}

	objs := make([]model.Obj, 0)
	for _, file := range bodyStruct.Data.FileList {
		objs = append(objs, &file)
	}

	return objs, nil
}

func (d *Pan123LinkDir) Link(ctx context.Context, file model.Obj, args model.LinkArgs) (*model.Link, error) {
	protocol := "http"
	if d.EnableHTTPS {
		protocol = "https"
	}
	var url string
	if d.UUID != "" {
		url = fmt.Sprintf("%s://%s/%s/%s", protocol, d.Domain, d.UUID, file.GetID())
	} else {
		url = fmt.Sprintf("%s://%s/%s", protocol, d.Domain, file.GetID())
	}

	return &model.Link{
		URL: url,
	}, nil
}

func (d *Pan123LinkDir) MakeDir(ctx context.Context, parentDir model.Obj, dirName string) (model.Obj, error) {
	url := OpenAPIBaseURL + "/upload/v1/file/mkdir"

	req := base.RestyClient.R().
		SetBody(map[string]string{
			"name":     dirName,
			"parentID": GetObjID(parentDir),
		}).
		SetHeader("Authorization", "Bearer "+d.access_token).
		SetHeader("Platform", "open_platform")

	res, err := req.Execute(http.MethodPost, url)
	if err != nil {
		return nil, err
	}

	body := res.Body()
	bodyStruct := struct {
		Data struct {
			DirID int `json:"dirID"`
		} `json:"data"`
	}{}

	_parentDir, err := strconv.Atoi(GetObjID(parentDir))
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(body, &bodyStruct)
	if err != nil {
		return nil, err
	}
	file := File{
		FileId:       bodyStruct.Data.DirID,
		FileName:     dirName,
		ParentFileId: int64(_parentDir),
		Type:         1,
	}

	return &file, nil
}

func (d *Pan123LinkDir) Move(ctx context.Context, srcObj, dstDir model.Obj) (model.Obj, error) {
	url := OpenAPIBaseURL + "/api/v1/file/move"

	req := base.RestyClient.R().
		SetBody(map[string]any{
			"fileIDs":        []any{GetObjID(srcObj)},
			"toParentFileID": GetObjID(dstDir),
		}).
		SetHeader("Authorization", "Bearer "+d.access_token).
		SetHeader("Platform", "open_platform")

	_, err := req.Execute(http.MethodPost, url)
	if err != nil {
		return nil, err
	}

	return srcObj, nil
}

func (d *Pan123LinkDir) Rename(ctx context.Context, srcObj model.Obj, newName string) (model.Obj, error) {
	url := OpenAPIBaseURL + "/api/v1/file/rename"

	req := base.RestyClient.R().
		SetBody(map[string]any{
			"renameList": []string{fmt.Sprintf("%s|%s", GetObjID(srcObj), newName)},
		}).
		SetHeader("Authorization", "Bearer "+d.access_token).
		SetHeader("Platform", "open_platform")

	_, err := req.Execute(http.MethodPost, url)
	if err != nil {
		return nil, err
	}

	objID, err := strconv.Atoi(GetObjID(srcObj))
	if err != nil {
		return nil, err
	}

	file := File{
		FileId:   objID,
		FileName: newName,
		Type:     0,
	}

	if srcObj.IsDir() {
		file.Type = 1
	}

	return &file, nil
}

func (d *Pan123LinkDir) Copy(ctx context.Context, srcObj, dstDir model.Obj) (model.Obj, error) {
	return nil, errs.NotImplement
}

func (d *Pan123LinkDir) Remove(ctx context.Context, obj model.Obj) error {
	url := OpenAPIBaseURL + "/api/v1/file/trash"
	url_delete := OpenAPIBaseURL + "/api/v1/file/delete"

	req := base.RestyClient.R().
		SetBody(map[string]any{
			"fileIDs": []any{obj.GetID()},
		}).
		SetHeader("Authorization", "Bearer "+d.access_token).
		SetHeader("Platform", "open_platform")

	_, err := req.Execute(http.MethodPost, url)
	if err != nil {
		return err
	}

	req_delete := base.RestyClient.R().
		SetBody(map[string]any{
			"fileIDs": []any{obj.GetID()},
		}).
		SetHeader("Authorization", "Bearer "+d.access_token).
		SetHeader("Platform", "open_platform")

	_, err = req_delete.Execute(http.MethodPost, url_delete)
	if err != nil {
		return err
	}

	return nil
}

func (d *Pan123LinkDir) Put(ctx context.Context, dstDir model.Obj, stream model.FileStreamer, up driver.UpdateProgress) (model.Obj, error) {
	return nil, errs.NotImplement
}

var _ driver.Driver = (*Pan123LinkDir)(nil)
