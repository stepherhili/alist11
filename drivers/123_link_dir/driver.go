package _123LinkDir

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

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
	req.SetHeader("Platform", "open_platform")
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
	parentFileID := getParentFileID(dir)

	req := base.RestyClient.R().
		SetQueryParam("parentFileId", parentFileID).
		SetQueryParam("limit", "100").
		SetHeader("Authorization", "Bearer "+d.access_token).
		SetHeader("Platform", "open_platform")

	res, err := req.Execute(http.MethodGet, url)
	if err != nil {
		return nil, fmt.Errorf("failed to get directory: %w", err)
	}

	body := res.Body()
	bodyStruct := struct {
		Data struct {
			FileList []File `json:"fileList"`
		} `json:"data"`
	}{}

	err = json.Unmarshal(body, &bodyStruct)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal directory objects: %w", err)
	}

	objs := make([]model.Obj, 0)
	for _, file := range bodyStruct.Data.FileList {
		objs = append(objs, &file)
	}

	return objs, nil
}

func (d *Pan123LinkDir) MakeDir(ctx context.Context, parentDir model.Obj, dirName string) (model.Obj, error) {
	parentFileID := getParentFileID(parentDir)

	if parentFileID == "" || parentFileID == "0" {
		return nil, fmt.Errorf("failed to get parent list: parent directory object not found")
	}

	url := OpenAPIBaseURL + "/upload/v1/file/mkdir"

	req := base.RestyClient.R().
		SetBody(map[string]string{
			"name":     dirName,
			"parentID": parentFileID,
		}).
		SetHeader("Authorization", "Bearer "+d.access_token).
		SetHeader("Platform", "open_platform")

	res, err := req.Execute(http.MethodPost, url)
	if err != nil {
		return nil, fmt.Errorf("failed to create directory: %w", err)
	}

	body := res.Body()
	bodyStruct := struct {
		Data struct {
			DirID int `json:"dirID"`
		} `json:"data"`
	}{}

	err = json.Unmarshal(body, &bodyStruct)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal mkdir response: %w", err)
	}

	file := File{
		FileId:       bodyStruct.Data.DirID,
		FileName:     dirName,
		ParentFileId: int64(bodyStruct.Data.DirID),
		Type:         1,
	}

	return &file, nil
}

func (d *Pan123LinkDir) Put(ctx context.Context, dstDir model.Obj, stream model.FileStreamer, up driver.UpdateProgress) (model.Obj, error) {
	meta := stream.Metadata()
	parentFileID := getParentFileID(dstDir)

	if parentFileID == "" || parentFileID == "0" {
		parentFileID = "0"
	}

	urlCreate := OpenAPIBaseURL + "/upload/v1/file/create"

	reqCreate := base.RestyClient.R().
		SetBody(map[string]interface{}{
			"parentFileID": parentFileID,
			"filename":     meta.Name,
			"etag":         meta.HashMD5,
			"size":         meta.Size,
		}).
		SetHeader("Authorization", "Bearer "+d.access_token).
		SetHeader("Platform", "open_platform")

	resCreate, err := reqCreate.Execute(http.MethodPost, urlCreate)
	if err != nil {
		return nil, fmt.Errorf("failed to create file: %w", err)
	}

	createResponse := struct {
		Data struct {
			Reuse      bool   `json:"reuse"`
			PreuploadID string `json:"preuploadID"`
			SliceSize  int    `json:"sliceSize"`
			FileID     string `json:"fileID"`
		} `json:"data"`
	}{}

	err = json.Unmarshal(resCreate.Body(), &createResponse)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal create file response: %w", err)
	}

	if createResponse.Data.Reuse {
		return &File{
			FileId:   createResponse.Data.FileID,
			FileName: meta.Name,
			Type:     0,
		}, nil
	}

	preuploadID := createResponse.Data.PreuploadID
	sliceSize := createResponse.Data.SliceSize
	fileParts := stream.Slice(sliceSize)

	for i, part := range fileParts {
		uploadURL := d.getUploadURL(preuploadID, i+1)
		err = d.uploadPart(uploadURL, part)
		if err != nil {
			return nil, fmt.Errorf("failed to upload part %d: %w", i+1, err)
		}
		up(i+1, len(fileParts))
	}

	urlComplete := OpenAPIBaseURL + "/upload/v1/file/upload_complete"
	reqComplete := base.RestyClient.R().
		SetBody(map[string]string{"preuploadID": preuploadID}).
		SetHeader("Authorization", "Bearer "+d.access_token).
		SetHeader("Platform", "open_platform")

	resComplete, err := reqComplete.Execute(http.MethodPost, urlComplete)
	if err != nil {
		return nil, fmt.Errorf("failed to complete upload: %w", err)
	}

	completeResponse := struct {
		Data struct {
			FileID    string `json:"fileID"`
			Completed bool   `json:"completed"`
			Async     bool   `json:"async"`
		} `json:"data"`
	}{}

	err = json.Unmarshal(resComplete.Body(), &completeResponse)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal upload complete response: %w", err)
	}

	if completeResponse.Data.Async {
		fileID, err := d.pollUploadResult(preuploadID)
		if err != nil {
			return nil, fmt.Errorf("failed to get async uoload result: %w", err)
		}
		completeResponse.Data.FileID = fileID
	}

	return &File{
		FileId:   completeResponse.Data.FileID,
		FileName: meta.Name,
		Type:     0,
	}, nil
}

// Helper to fetch parentFileId recursively
func getParentFileID(obj model.Obj) string {
	if obj == nil || obj.GetID() == "" || obj.GetID() == "0" {
		return "0"
	}

	fileID := obj.GetID()
	pathParts := []string{fileID}

	parent := obj.GetParent()
	for parent != nil && parent.GetID() != "" {
		pathParts = append([]string{parent.GetID()}, pathParts...)
		parent = parent.GetParent()
	}

	if len(pathParts) == 0 {
		return "0"
	}

	return pathParts[len(pathParts)-1]
}

func (d *Pan123LinkDir) getUploadURL(preuploadID string, sliceNo int) string {
	urlGetUpload := OpenAPIBaseURL + "/upload/v1/file/get_upload_url"

	req := base.RestyClient.R().
		SetBody(map[string]interface{}{
			"preuploadID": preuploadID,
			"sliceNo":     sliceNo,
		}).
		SetHeader("Authorization", "Bearer "+d.access_token).
		SetHeader("Platform", "open_platform")

	res, err := req.Execute(http.MethodPost, urlGetUpload)
	if err != nil {
		panic(fmt.Sprintf("failed to get upload URL for slice %d: %v", sliceNo, err))
	}

	response := struct {
		Data struct {
			PresignedURL string `json:"presignedURL"`
		} `json:"data"`
	}{}

	err = json.Unmarshal(res.Body(), &response)
	if err != nil {
		panic(fmt.Sprintf("failed to unmarshal get upload URL response: %v", err))
	}

	return response.Data.PresignedURL
}

func (d *Pan123LinkDir) uploadPart(url string, part []byte) error {
	req := base.RestyClient.R().SetHeader("Content-Type", "application/octet-stream").SetBody(part)
	_, err := req.Execute(http.MethodPut, url)
	return err
}

func (d *Pan123LinkDir) pollUploadResult(preuploadID string) (string, error) {
	urlAsync := OpenAPIBaseURL + "/upload/v1/file/upload_async_result"

	for {
		req := base.RestyClient.R().
			SetBody(map[string]string{"preuploadID": preuploadID}).
			SetHeader("Authorization", "Bearer "+d.access_token).
			SetHeader("Platform", "open_platform")

		res, err := req.Execute(http.MethodPost, urlAsync)
		if err != nil {
			return "", err
		}

		response := struct {
			Data struct {
				Completed bool   `json:"completed"`
				FileID    string `json:"fileID"`
			} `json:"data"`
		}{}

		err = json.Unmarshal(res.Body(), &response)
		if err != nil {
			return "", err
		}

		if response.Data.Completed {
			return response.Data.FileID, nil
		}

		time.Sleep(1 * time.Second)
	}
}

var _ driver.Driver = (*Pan123LinkDir)(nil)
