package _123LinkDir

import (
	"bytes"
        "context"
        "encoding/json"
        "fmt"
        "io"
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
    parentFileID := GetObjID(dstDir)

    // Manually extract filename, MD5, and size for the upload
    filename, err := getFilenameFromStream(stream)
    if err != nil {
        return nil, fmt.Errorf("failed to get filename: %w", err)
    }
    etag, err := calculateMD5(stream)
    if err != nil {
        return nil, fmt.Errorf("failed to calculate MD5: %w", err)
    }
    size := getSizeFromStream(stream)

    // Step 1: Create File
    createURL := DIRVER_API + "/upload/v1/file/create"
    req := base.RestyClient.R().
        SetBody(map[string]any{
            "parentFileID": parentFileID,
            "filename":     filename,
            "etag":         etag,
            "size":         size,
            "duplicate":    1, // choose the strategy that suits your needs
        }).
        SetHeader("Authorization", "Bearer "+d.access_token).
        SetHeader("Platform", "open_platform")

    res, err := req.Execute(http.MethodPost, createURL)
    if err != nil {
        return nil, fmt.Errorf("failed to create file: %w", err)
    }

    body := res.Body()
    createResp := struct {
        Data struct {
            FileID      int    `json:"fileID"`
            PreuploadID string `json:"preuploadID"`
            Reuse       bool   `json:"reuse"`
            SliceSize   int    `json:"sliceSize"`
        } `json:"data"`
    }{}
    err = json.Unmarshal(body, &createResp)
    if err != nil {
        return nil, fmt.Errorf("failed to parse create file response: %w", err)
    }

    if createResp.Data.Reuse {
        // File already exists (instant upload), return the file object
        return &File{
            FileId:   createResp.Data.FileID,
            FileName: filename,
            Size:     size,
            MD5:      etag,
        }, nil
    }

    preuploadID := createResp.Data.PreuploadID
    sliceSize := createResp.Data.SliceSize

    // Step 2: Upload Parts
    sliceNo := 1
    buffer := make([]byte, sliceSize)
    for {
        n, err := stream.Read(buffer)
        if err != nil && err != io.EOF {
            return nil, fmt.Errorf("failed to read file slice: %w", err)
        }

        if n == 0 {
            break // End of file stream
        }

        // Get upload URL for each slice
        uploadURLReq := base.RestyClient.R().
            SetBody(map[string]any{
                "preuploadID": preuploadID,
                "sliceNo":     sliceNo,
            }).
            SetHeader("Authorization", "Bearer "+d.access_token).
            SetHeader("Platform", "open_platform")

        uploadURLRes, err := uploadURLReq.Execute(http.MethodPost, DIRVER_API+"/upload/v1/file/get_upload_url")
        if err != nil {
            return nil, fmt.Errorf("failed to get upload URL: %w", err)
        }

        uploadURLResp := struct {
            Data struct {
                PresignedURL string `json:"presignedURL"`
            } `json:"data"`
        }{}
        err = json.Unmarshal(uploadURLRes.Body(), &uploadURLResp)
        if err != nil {
            return nil, fmt.Errorf("failed to parse get upload URL response: %w", err)
        }

        // Upload the current slice
        _, err = http.Post(uploadURLResp.Data.PresignedURL, "application/octet-stream", bytes.NewReader(buffer[:n]))
        if err != nil {
            return nil, fmt.Errorf("failed to upload slice %d: %w", sliceNo, err)
        }

        sliceNo++
    }

    // Step 3: Check uploaded parts
    checkPartsReq := base.RestyClient.R().
        SetBody(map[string]any{
            "preuploadID": preuploadID,
        }).
        SetHeader("Authorization", "Bearer "+d.access_token).
        SetHeader("Platform", "open_platform")

    checkPartsRes, err := checkPartsReq.Execute(http.MethodPost, DIRVER_API+"/upload/v1/file/list_upload_parts")
    if err != nil {
        return nil, fmt.Errorf("failed to list uploaded parts: %w", err)
    }

    checkPartsResp := struct {
        Data struct {
            Parts []struct {
                PartNumber int    `json:"partNumber"`
                Size       int    `json:"size"`
                ETag       string `json:"etag"`
            } `json:"parts"`
        } `json:"data"`
    }{}
    err = json.Unmarshal(checkPartsRes.Body(), &checkPartsResp)
    if err != nil {
        return nil, fmt.Errorf("failed to parse list uploaded parts response: %w", err)
    }

    // Optional: Implement MD5 verification between local and cloud parts if needed

    // Step 4: Complete Upload
    completeReq := base.RestyClient.R().
        SetBody(map[string]any{
            "preuploadID": preuploadID,
        }).
        SetHeader("Authorization", "Bearer "+d.access_token).
        SetHeader("Platform", "open_platform")

    completeRes, err := completeReq.Execute(http.MethodPost, DIRVER_API+"/upload/v1/file/upload_complete")
    if err != nil {
        return nil, fmt.Errorf("failed to complete upload: %w", err)
    }

    completeBody := completeRes.Body()
    completeResp := struct {
        Data struct {
            FileID    int  `json:"fileID"`
            Async     bool `json:"async"`
            Completed bool `json:"completed"`
        } `json:"data"`
    }{}
    err = json.Unmarshal(completeBody, &completeResp)
    if err != nil {
        return nil, fmt.Errorf("failed to parse complete upload response: %w", err)
    }

    // Check if upload is completed or needs async polling
    if completeResp.Data.Completed {
        return &File{
            FileId:   completeResp.Data.FileID,
            FileName: filename,
            Size:     size,
            MD5:      etag,
        }, nil
    }

    // Step 5: Async Polling if needed
    for completeResp.Data.Async && !completeResp.Data.Completed {
        // Add delay between polling
        time.Sleep(time.Second)

        asyncReq := base.RestyClient.R().
            SetBody(map[string]any{
                "preuploadID": preuploadID,
            }).
            SetHeader("Authorization", "Bearer "+d.access_token).
            SetHeader("Platform", "open_platform")

        asyncRes, err := asyncReq.Execute(http.MethodPost, DIRVER_API+"/upload/v1/file/upload_async_result")
        if err != nil {
            return nil, fmt.Errorf("failed to get async result: %w", err)
        }

        asyncResp := struct {
            Data struct {
                Completed bool `json:"completed"`
                FileID    int  `json:"fileID"`
            } `json:"data"`
        }{}

        err = json.Unmarshal(asyncRes.Body(), &asyncResp)
        if err != nil {
            return nil, fmt.Errorf("failed to parse async result response: %w", err)
        }

        if asyncResp.Data.Completed {
            return &File{
                FileId:   asyncResp.Data.FileID,
                FileName: filename,
                Size:     size,
                MD5:      etag,
            }, nil
        }
    }

    return nil, fmt.Errorf("upload could not be completed")
}

// Helper function to calculate MD5 from stream
func calculateMD5(stream model.FileStreamer) (string, error) {
	// Implement the MD5 calculation logic here
	return "dummy-md5", nil // please replace this with actual computation
}

// Helper function to get filename from the streamer
func getFilenameFromStream(stream model.FileStreamer) (string, error) {
	// Implement the logic to get filename from stream
	return "dummy-filename", nil // please replace this with actual logic
}

// Helper function to get size of file from the streamer
func getSizeFromStream(stream model.FileStreamer) int64 {
	// Implement the logic to for getting size. Assumes a function exists.
	return 123456 // please replace with actual logic
}
var _ driver.Driver = (*Pan123LinkDir)(nil)
