package service

import (
	"molly-server/internal/config"
	"molly-server/pkg/objectstorage"
)

type service struct {
	repo       Repository
	uploadCfg  config.UploadConfig
	storageCfg config.ObjectStorageConfig
	storage    objectstorage.Client
}

func New(
	repo Repository,
	uploadCfg config.UploadConfig,
	storageCfg config.ObjectStorageConfig,
	storage objectstorage.Client,
) Service {
	return &service{
		repo:       repo,
		uploadCfg:  uploadCfg,
		storageCfg: storageCfg,
		storage:    storage,
	}
}
