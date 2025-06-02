package _115_open

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/pkg/utils"
	"github.com/aliyun/aliyun-oss-go-sdk/oss"
	"github.com/avast/retry-go"
	sdk "github.com/xhofe/115-sdk-go"
)

func calPartSize(fileSize int64) int64 {
	var partSize int64 = 20 * utils.MB
	if fileSize > partSize {
		if fileSize > 1*utils.TB { // file Size over 1TB
			partSize = 5 * utils.GB // file part size 5GB
		} else if fileSize > 768*utils.GB { // over 768GB
			partSize = 109951163 // ≈ 104.8576MB, split 1TB into 10,000 part
		} else if fileSize > 512*utils.GB { // over 512GB
			partSize = 82463373 // ≈ 78.6432MB
		} else if fileSize > 384*utils.GB { // over 384GB
			partSize = 54975582 // ≈ 52.4288MB
		} else if fileSize > 256*utils.GB { // over 256GB
			partSize = 41231687 // ≈ 39.3216MB
		} else if fileSize > 128*utils.GB { // over 128GB
			partSize = 27487791 // ≈ 26.2144MB
		}
	}
	return partSize
}

func (d *Open115) singleUpload(ctx context.Context, tempF model.File, tokenResp *sdk.UploadGetTokenResp, initResp *sdk.UploadInitResp) error {
	ossClient, err := oss.New(tokenResp.Endpoint, tokenResp.AccessKeyId, tokenResp.AccessKeySecret, oss.SecurityToken(tokenResp.SecurityToken))
	if err != nil {
		return err
	}
	bucket, err := ossClient.Bucket(initResp.Bucket)
	if err != nil {
		return err
	}

	err = bucket.PutObject(initResp.Object, tempF,
		oss.Callback(base64.StdEncoding.EncodeToString([]byte(initResp.Callback.Value.Callback))),
		oss.CallbackVar(base64.StdEncoding.EncodeToString([]byte(initResp.Callback.Value.CallbackVar))),
	)

	return err
}

// type CallbackResult struct {
// 	State   bool   `json:"state"`
// 	Code    int    `json:"code"`
// 	Message string `json:"message"`
// 	Data    struct {
// 		PickCode string `json:"pick_code"`
// 		FileName string `json:"file_name"`
// 		FileSize int64  `json:"file_size"`
// 		FileID   string `json:"file_id"`
// 		ThumbURL string `json:"thumb_url"`
// 		Sha1     string `json:"sha1"`
// 		Aid      int    `json:"aid"`
// 		Cid      string `json:"cid"`
// 	} `json:"data"`
// }

func (d *Open115) isTokenExpired(tokenResp *sdk.UploadGetTokenResp) bool {
	// 解析过期时间字符串
	expiration, err := time.Parse(time.RFC3339, tokenResp.Expiration)
	if err != nil {
		// 如果解析失败，保守起见认为token已过期
		return true
	}
	// 在20分钟时刷新token，而不是等到快过期时
	refreshTime := time.Now().Add(20 * time.Minute)
	return expiration.Before(refreshTime)
}

func (d *Open115) refreshUploadToken(ctx context.Context) (*sdk.UploadGetTokenResp, error) {
	return d.client.UploadGetToken(ctx)
}

func (d *Open115) multpartUpload(ctx context.Context, tempF model.File, stream model.FileStreamer, up driver.UpdateProgress, tokenResp *sdk.UploadGetTokenResp, initResp *sdk.UploadInitResp) error {
	fileSize := stream.GetSize()
	chunkSize := calPartSize(fileSize)

	createOSSClient := func(token *sdk.UploadGetTokenResp) (*oss.Bucket, error) {
		ossClient, err := oss.New(token.Endpoint, token.AccessKeyId, token.AccessKeySecret, oss.SecurityToken(token.SecurityToken))
		if err != nil {
			return nil, err
		}
		return ossClient.Bucket(initResp.Bucket)
	}

	bucket, err := createOSSClient(tokenResp)
	if err != nil {
		return err
	}

	imur, err := bucket.InitiateMultipartUpload(initResp.Object, oss.Sequential())
	if err != nil {
		return err
	}

	partNum := (stream.GetSize() + chunkSize - 1) / chunkSize
	parts := make([]oss.UploadPart, partNum)
	offset := int64(0)

	lastTokenCheck := time.Now()
	// 每5分钟检查一次token状态
	tokenCheckInterval := 5 * time.Minute
	// 记录token创建时间
	tokenCreateTime := time.Now()

	for i := int64(1); i <= partNum; i++ {
		if utils.IsCanceled(ctx) {
			return ctx.Err()
		}

		// 检查是否需要刷新token
		needRefresh := false
		if time.Since(lastTokenCheck) > tokenCheckInterval {
			// 如果距离上次检查超过5分钟，检查token状态
			if d.isTokenExpired(tokenResp) {
				needRefresh = true
			}
			lastTokenCheck = time.Now()
		} else if time.Since(tokenCreateTime) > 20*time.Minute {
			// 如果token创建时间超过20分钟，主动刷新
			needRefresh = true
		}

		if needRefresh {
			newToken, err := d.refreshUploadToken(ctx)
			if err != nil {
				return err
			}
			tokenResp = newToken
			tokenCreateTime = time.Now() // 更新token创建时间
			bucket, err = createOSSClient(tokenResp)
			if err != nil {
				return err
			}
			// 重新初始化分片上传
			imur, err = bucket.InitiateMultipartUpload(initResp.Object, oss.Sequential())
			if err != nil {
				return err
			}
		}

		partSize := chunkSize
		if i == partNum {
			partSize = fileSize - (i-1)*chunkSize
		}
		rd := utils.NewMultiReadable(io.LimitReader(stream, partSize))
		err = retry.Do(func() error {
			_ = rd.Reset()
			rateLimitedRd := driver.NewLimitedUploadStream(ctx, rd)
			part, err := bucket.UploadPart(imur, rateLimitedRd, partSize, int(i))
			if err != nil {
				// 检查是否是token过期错误
				if strings.Contains(err.Error(), "SecurityTokenExpired") {
					// 立即刷新token
					newToken, err := d.refreshUploadToken(ctx)
					if err != nil {
						return err
					}
					tokenResp = newToken
					tokenCreateTime = time.Now() // 更新token创建时间
					bucket, err = createOSSClient(tokenResp)
					if err != nil {
						return err
					}
					// 重新初始化分片上传
					imur, err = bucket.InitiateMultipartUpload(initResp.Object, oss.Sequential())
					if err != nil {
						return err
					}
					// 返回特殊错误以触发重试
					return fmt.Errorf("token expired, retry with new token")
				}
				return err
			}
			parts[i-1] = part
			return nil
		},
			retry.Attempts(3),
			retry.DelayType(retry.BackOffDelay),
			retry.Delay(time.Second),
			retry.If(func(err error) bool {
				return strings.Contains(err.Error(), "token expired, retry with new token")
			}))
		if err != nil {
			return err
		}

		if i == partNum {
			offset = fileSize
		} else {
			offset += partSize
		}
		up(float64(offset) / float64(fileSize))
	}

	_, err = bucket.CompleteMultipartUpload(
		imur,
		parts,
		oss.Callback(base64.StdEncoding.EncodeToString([]byte(initResp.Callback.Value.Callback))),
		oss.CallbackVar(base64.StdEncoding.EncodeToString([]byte(initResp.Callback.Value.CallbackVar))),
	)
	if err != nil {
		return err
	}

	return nil
}
