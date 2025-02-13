package _123LinkDir

import (
	"strconv"
	"time"

	"github.com/alist-org/alist/v3/pkg/utils"
)

type File struct {
	FileId       int    `json:"fileId"`
	FileName     string `json:"filename"`
	Type         int    `json:"type"`
	Size         int64  `json:"size"`
	MD5          string `json:"etag"`
	Status       int    `json:"status"`
	ParentFileId int64  `json:"parentFileId"`
	Category     int    `json:"category"`
}

func (f *File) GetSize() int64 {
	return f.Size
}

func (f *File) GetName() string {
	return f.FileName
}

func (f *File) ModTime() time.Time {
	return time.Now()
}

func (f *File) CreateTime() time.Time {
	return time.Now()
}

func (f *File) IsDir() bool {
	return f.Type == 1
}

func (f *File) GetHash() utils.HashInfo {
	return utils.NewHashInfo(nil, f.MD5)
}

func (f *File) GetID() string {
	return strconv.Itoa(f.FileId)
}

func (f *File) GetPath() string {
	return f.FileName
}
