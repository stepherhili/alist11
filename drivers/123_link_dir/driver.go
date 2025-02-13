package _123LinkDir

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/alist-org/alist/v3/drivers/base"
	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/model"
)

// 统一定义 OpenAPIBaseURL
//const OpenAPIBaseURL = "https://open-api.123pan.com"

type Pan123LinkDir struct {
	model.Storage
	Addition
	access_token string
}

type FileMetadata struct {
	Name   string
	HashMD5 string
	Size   int64
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
	rootFolderID := strconv.Itoa(d.Addition.RootFolderID)

	// 递归获取目标目录的 parentFileId
	parentFileId, err := d.getParentFileId(ctx, dir.GetID(), rootFolderID)
	if err != nil {
		return nil, fmt.Errorf("failed to get parent file ID: %w", err)
	}

	// 获取文件列表
	files, err := d.getFileList(ctx, parentFileId)
	if err != nil {
		return nil, fmt.Errorf("failed to get file list: %w", err)
	}

	// 转换文件列表为 []model.Obj
	return utils.SliceConvert(files, func(src File) (model.Obj, error) {
		return src, nil
	})
}

// getParentFileId 递归获取指定目录的 parentFileId
func (d *Pan123LinkDir) getParentFileId(ctx context.Context, currentFileId string, rootFolderID string) (string, error) {
	fileInfo, err := d.getFileInfo(ctx, currentFileId)
	if err != nil {
		return "", fmt.Errorf("could not get file info for ID %s: %w", currentFileId, err)
	}

	// 检查是否到达根目录
	if strconv.Itoa(fileInfo.ParentFileId) == rootFolderID {
		return currentFileId, nil // 返回当前文件 ID
	}

	// 递归查找父级目录
	return d.getParentFileId(ctx, strconv.Itoa(fileInfo.ParentFileId), rootFolderID)
}

// getFileInfo 获取指定 ID 文件的信息
func (d *Pan123LinkDir) getFileInfo(ctx context.Context, fileId string) (File, error) {
	var resp FileResponse
	_res, err := d.Request(fmt.Sprintf("/api/v2/file/get?id=%s", fileId), http.MethodGet, func(req *resty.Request) {
		req.SetHeader("Authorization", "Bearer "+d.access_token)
		req.SetHeader("Platform", "open_platform")
	})

	if err != nil {
		return File{}, fmt.Errorf("request failed: %w", err)
	}

	if err := json.Unmarshal(_res.Body(), &resp); err != nil {
		return File{}, fmt.Errorf("failed to parse response: %w", err)
	}

	return resp.Data, nil
}

// getFileList 根据 parentFileId 获取文件列表
func (d *Pan123LinkDir) getFileList(ctx context.Context, parentFileId string) ([]File, error) {
	var res []File
	page := 1

	for {
		if err := d.APIRateLimit(ctx, FileList); err != nil {
			return nil, fmt.Errorf("rate limit exceeded: %w", err)
		}

		var resp Files
		query := map[string]string{
			"parentFileId": parentFileId,
			"limit":        "100", // 每页文件数量，最大不超过100
		}

		_res, err := d.Request("/api/v2/file/list", http.MethodGet, func(req *resty.Request) {
			req.SetHeader("Authorization", "Bearer "+d.access_token)
			req.SetHeader("Platform", "open_platform")
			req.SetQueryParams(query)
		}, &resp)

		if err != nil {
			return nil, fmt.Errorf("request for file list failed: %w", err)
		}

		res = append(res, resp.Data.InfoList...)

		if len(resp.Data.InfoList) == 0 {
			break // 没有更多文件，退出循环
		}

		page++ // 下一页
	}

	return res, nil
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

func (d *Pan123LinkDir) Put(ctx context.Context, dstDir model.Obj, stream model.FileStreamer, up func(float64)) (model.Obj, error) {
	meta := getFileMetadata(stream) // 使用辅助函数提取元数据
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
			Reuse       bool   `json:"reuse"`
			PreuploadID string `json:"preuploadID"`
			SliceSize   int    `json:"sliceSize"`
			FileID      int    `json:"fileID"`
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
	fileParts := sliceFile(stream, sliceSize) // 使用辅助函数进行分片

	for i, part := range fileParts {
		uploadURL := d.getUploadURL(preuploadID, i+1)
		err = d.uploadPart(uploadURL, part)
		if err != nil {
			return nil, fmt.Errorf("failed to upload part %d: %w", i+1, err)
		}
		up(float64(i+1) / float64(len(fileParts)))
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
			FileID    int    `json:"fileID"`
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
			return nil, fmt.Errorf("failed to get async upload result: %w", err)
		}
		completeResponse.Data.FileID = fileID
	}

	return &File{
		FileId:   completeResponse.Data.FileID,
		FileName: meta.Name,
		Type:     0,
	}, nil
}

func getParentFileID(obj model.Obj) string {
	if obj == nil || obj.GetID() == "" {
		return "0"
	}
	return obj.GetID() // 假设 obj 的 ID 就是 parentFileID
}

// 从 FileStreamer 中提取元数据的辅助函数
func getFileMetadata(stream model.FileStreamer) *FileMetadata {
	// 实现逻辑以提取流的元数据
	return &FileMetadata{
		Name:   "SampleName", // 这里需要实际从 stream 取得
		HashMD5: "SampleHashMD5", // 需要实际从 stream 取得
		Size:   12345, // 需要实际从 stream 取得
	}
}

// 分片文件的辅助函数
func sliceFile(stream model.FileStreamer, sliceSize int) [][]byte {
	// 实现分片逻辑
	return [][]byte{}
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

func (d *Pan123LinkDir) pollUploadResult(preuploadID string) (int, error) {
	urlAsync := OpenAPIBaseURL + "/upload/v1/file/upload_async_result"

	for {
		req := base.RestyClient.R().
			SetBody(map[string]string{"preuploadID": preuploadID}).
			SetHeader("Authorization", "Bearer "+d.access_token).
			SetHeader("Platform", "open_platform")

		res, err := req.Execute(http.MethodPost, urlAsync)
		if err != nil {
			return 0, err
		}

		response := struct {
			Data struct {
				Completed bool `json:"completed"`
				FileID    int  `json:"fileID"`
			} `json:"data"`
		}{}

		err = json.Unmarshal(res.Body(), &response)
		if err != nil {
			return 0, err
		}

		if response.Data.Completed {
			return response.Data.FileID, nil
		}

		time.Sleep(1 * time.Second)
	}
}

var _ driver.Driver = (*Pan123LinkDir)(nil)
