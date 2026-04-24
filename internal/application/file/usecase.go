package file

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	domainfile "molly-server/internal/domain/file"
	domainuser "molly-server/internal/domain/user"
	"molly-server/pkg/cache"
)

const (
	defaultChunkSize   = int64(5 << 20) // 5 MiB
	uploadTaskTTL      = 7 * 24 * time.Hour
	precheckCacheTTL   = 12 * 60 * 60 // 12h in seconds
	uploadLockCacheTTL = 24 * 60 * 60 // 24h in seconds
)

// Deps 依赖集合，通过结构体注入避免构造函数参数爆炸。
type Deps struct {
	FileInfo    domainfile.FileInfoRepository
	UserFile    domainfile.UserFileRepository
	VirtualPath domainfile.VirtualPathRepository
	UploadTask  domainfile.UploadTaskRepository
	UserRepo    domainuser.Repository // 读取用户空间配额
	Cache       cache.Cache
	StoragePath string // 文件存储根路径
	// MoveToRecycled 由 recycled/UseCase 提供，解耦两个域的依赖方向。
	// file 域不直接依赖 recycled 域，通过函数注入实现协作。
	MoveToRecycled func(ctx context.Context, userID, userFileID string) error
}

type UseCase struct {
	deps Deps
	// 进行中的文件合并锁，key = precheck_id，防止同一任务被并发触发多次合并
	mergeLocks sync.Map
}

func NewUseCase(deps Deps) *UseCase {
	return &UseCase{deps: deps}
}

// ── 预检与秒传 ────────────────────────────────────────────────

// Precheck 上传预检：检查空间、尝试秒传、创建任务记录。
// 返回 PrecheckResp.AlreadyDone=true 时客户端无需再上传。
func (uc *UseCase) Precheck(ctx context.Context, userID string, req PrecheckReq) (*PrecheckResp, error) {
	// 1. 检查用户存储配额
	user, err := uc.deps.UserRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if user.Space > 0 && user.FreeSpace < req.FileSize {
		return nil, domainfile.ErrInsufficientSpace
	}

	// 2. 尝试秒传：通过分片签名快速定位候选文件
	if req.ChunkSignature != "" {
		if resp, ok := uc.tryInstantUpload(ctx, user, req); ok {
			return resp, nil
		}
	}

	// 3. 创建上传任务（幂等：同 precheck_id 可重复调用）
	precheckID := uuid.NewString()
	totalChunks := int((req.FileSize + defaultChunkSize - 1) / defaultChunkSize)

	task := &domainfile.UploadTask{
		ID:             precheckID,
		UserID:         userID,
		FileName:       req.FileName,
		FileSize:       req.FileSize,
		ChunkSize:      defaultChunkSize,
		TotalChunks:    totalChunks,
		ChunkSignature: req.ChunkSignature,
		PathID:         req.PathID,
		Status:         domainfile.UploadStatusPending,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
		ExpiresAt:      time.Now().Add(uploadTaskTTL),
	}
	if err := uc.deps.UploadTask.Create(ctx, task); err != nil {
		return nil, fmt.Errorf("precheck: create task: %w", err)
	}

	// 4. 把 req 序列化存缓存，上传阶段取用（统一 JSON，不区分 cache 类型）
	if err := uc.cacheSet(precheckCacheKey(precheckID), req, precheckCacheTTL); err != nil {
		return nil, fmt.Errorf("precheck: cache req: %w", err)
	}

	return &PrecheckResp{
		PrecheckID:  precheckID,
		AlreadyDone: false,
	}, nil
}

// tryInstantUpload 尝试秒传，成功返回 (resp, true)，否则 (nil, false)。
func (uc *UseCase) tryInstantUpload(ctx context.Context, user *domainuser.User, req PrecheckReq) (*PrecheckResp, bool) {
	candidate, err := uc.deps.FileInfo.GetByChunkSignature(ctx, req.ChunkSignature, req.FileSize)
	if err != nil || candidate == nil || candidate.IsEnc {
		return nil, false
	}

	// 三分片 MD5 比对（文件较大时）或全量 hash 比对（文件较小时）
	matched := false
	if len(req.FilesMd5) >= 3 {
		matched = candidate.FirstChunkHash == req.FilesMd5[0] &&
			candidate.SecondChunkHash == req.FilesMd5[1] &&
			candidate.ThirdChunkHash == req.FilesMd5[2]
	} else if len(req.FilesMd5) == 1 {
		matched = candidate.FileHash == req.FilesMd5[0]
	}
	if !matched {
		return nil, false
	}

	// 创建用户文件关联
	uf := &domainfile.UserFile{
		ID:          uuid.NewString(),
		UserID:      user.ID,
		FileID:      candidate.ID,
		FileName:    req.FileName,
		VirtualPath: req.PathID,
		IsPublic:    false,
		CreatedAt:   time.Now(),
	}
	if err := uc.deps.UserFile.Create(ctx, uf); err != nil {
		return nil, false
	}

	// 扣除配额
	if user.Space > 0 {
		user.FreeSpace -= req.FileSize
		if err := uc.deps.UserRepo.Update(ctx, user); err != nil {
			// 回滚：删除刚创建的 UserFile
			_ = uc.deps.UserFile.SoftDelete(ctx, uf.ID)
			return nil, false
		}
	}

	return &PrecheckResp{PrecheckID: "", AlreadyDone: true}, true
}

// ── 文件上传 ──────────────────────────────────────────────────

// UploadChunk 处理单次 HTTP 上传请求（可能是完整文件或单个分片）。
func (uc *UseCase) UploadChunk(
	ctx context.Context,
	userID string,
	req UploadChunkReq,
	file multipart.File,
	header *multipart.FileHeader,
) (*UploadResp, error) {
	// 1. 读取预检信息
	var precheckReq PrecheckReq
	if err := uc.cacheGet(precheckCacheKey(req.PrecheckID), &precheckReq); err != nil {
		return nil, domainfile.ErrPrecheckExpired
	}

	// 2. 准备临时目录
	tempDir := filepath.Join(uc.deps.StoragePath, "temp",
		fmt.Sprintf("%s_%s", sanitize(precheckReq.FileName), req.PrecheckID[:8]))
	if err := os.MkdirAll(tempDir, 0o755); err != nil {
		return nil, fmt.Errorf("upload: mkdir temp: %w", err)
	}

	isChunk := req.ChunkIndex != nil && req.TotalChunks != nil

	if isChunk {
		return uc.handleChunk(ctx, userID, req, file, header, tempDir, precheckReq)
	}
	return uc.handleSingleFile(ctx, userID, req, file, header, tempDir, precheckReq)
}

func (uc *UseCase) handleChunk(
	ctx context.Context,
	userID string,
	req UploadChunkReq,
	file multipart.File,
	header *multipart.FileHeader,
	tempDir string,
	precheckReq PrecheckReq,
) (*UploadResp, error) {
	chunkIndex := *req.ChunkIndex
	totalChunks := *req.TotalChunks

	// 保存分片
	chunkPath := filepath.Join(tempDir, fmt.Sprintf("%d.chunk", chunkIndex))
	if err := saveToFile(file, chunkPath); err != nil {
		return nil, fmt.Errorf("upload: save chunk %d: %w", chunkIndex, err)
	}

	// 统计已到位的分片数（以磁盘文件为准，不依赖 DB）
	uploaded := countChunks(tempDir, totalChunks)

	// 更新任务进度
	_ = uc.updateTaskProgress(ctx, req.PrecheckID, uploaded, totalChunks, tempDir, domainfile.UploadStatusUploading, "")

	if uploaded < totalChunks {
		return &UploadResp{IsComplete: false, Uploaded: uploaded, Total: totalChunks}, nil
	}

	// 所有分片到齐，触发合并（幂等保护：同一 precheckID 只合并一次）
	if _, loaded := uc.mergeLocks.LoadOrStore(req.PrecheckID, struct{}{}); loaded {
		return &UploadResp{IsComplete: false, Uploaded: uploaded, Total: totalChunks}, nil
	}
	defer uc.mergeLocks.Delete(req.PrecheckID)

	fileID, err := uc.mergeAndSave(ctx, userID, req, tempDir, totalChunks, precheckReq)
	if err != nil {
		_ = uc.updateTaskProgress(ctx, req.PrecheckID, uploaded, totalChunks, tempDir, domainfile.UploadStatusFailed, err.Error())
		return nil, err
	}

	_ = uc.updateTaskProgress(ctx, req.PrecheckID, totalChunks, totalChunks, tempDir, domainfile.UploadStatusCompleted, "")
	uc.cleanPrecheckCache(req.PrecheckID)

	return &UploadResp{FileID: fileID, IsComplete: true}, nil
}

func (uc *UseCase) handleSingleFile(
	ctx context.Context,
	userID string,
	req UploadChunkReq,
	file multipart.File,
	header *multipart.FileHeader,
	tempDir string,
	precheckReq PrecheckReq,
) (*UploadResp, error) {
	tmpPath := filepath.Join(tempDir, "upload.tmp")
	if err := saveToFile(file, tmpPath); err != nil {
		return nil, fmt.Errorf("upload: save file: %w", err)
	}

	destName := uuid.NewString()
	destDir := filepath.Join(uc.deps.StoragePath, "files")
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return nil, fmt.Errorf("upload: mkdir dest: %w", err)
	}
	destPath := filepath.Join(destDir, destName)
	if err := os.Rename(tmpPath, destPath); err != nil {
		return nil, fmt.Errorf("upload: move file: %w", err)
	}
	defer os.RemoveAll(tempDir)

	mime := header.Header.Get("Content-Type")
	if mime == "" {
		mime = "application/octet-stream"
	}

	fileInfo := &domainfile.FileInfo{
		ID:             uuid.NewString(),
		Name:           header.Filename,
		RandomName:     destName,
		Size:           header.Size,
		Mime:           mime,
		Path:           destPath,
		ChunkSignature: precheckReq.ChunkSignature,
		IsEnc:          req.IsEnc,
		IsChunk:        false,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	if len(precheckReq.FilesMd5) > 0 {
		fileInfo.FirstChunkHash = precheckReq.FilesMd5[0]
	}
	if len(precheckReq.FilesMd5) > 1 {
		fileInfo.SecondChunkHash = precheckReq.FilesMd5[1]
	}
	if len(precheckReq.FilesMd5) > 2 {
		fileInfo.ThirdChunkHash = precheckReq.FilesMd5[2]
	}

	if err := uc.deps.FileInfo.Create(ctx, fileInfo); err != nil {
		return nil, fmt.Errorf("upload: create file_info: %w", err)
	}

	uf := &domainfile.UserFile{
		ID:          uuid.NewString(),
		UserID:      userID,
		FileID:      fileInfo.ID,
		FileName:    precheckReq.FileName,
		VirtualPath: precheckReq.PathID,
		CreatedAt:   time.Now(),
	}
	if err := uc.deps.UserFile.Create(ctx, uf); err != nil {
		return nil, fmt.Errorf("upload: create user_file: %w", err)
	}

	uc.cleanPrecheckCache(req.PrecheckID)
	return &UploadResp{FileID: uf.ID, IsComplete: true}, nil
}

// mergeAndSave 合并所有分片，写入持久化存储，创建 FileInfo 和 UserFile 记录。
func (uc *UseCase) mergeAndSave(
	ctx context.Context,
	userID string,
	req UploadChunkReq,
	tempDir string,
	totalChunks int,
	precheckReq PrecheckReq,
) (string, error) {
	destName := uuid.NewString()
	destDir := filepath.Join(uc.deps.StoragePath, "files")
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return "", fmt.Errorf("merge: mkdir: %w", err)
	}
	destPath := filepath.Join(destDir, destName)

	out, err := os.Create(destPath)
	if err != nil {
		return "", fmt.Errorf("merge: create dest: %w", err)
	}
	defer out.Close()

	for i := 0; i < totalChunks; i++ {
		chunkPath := filepath.Join(tempDir, fmt.Sprintf("%d.chunk", i))
		chunk, err := os.Open(chunkPath)
		if err != nil {
			return "", fmt.Errorf("merge: open chunk %d: %w", i, err)
		}
		_, err = io.Copy(out, chunk)
		chunk.Close()
		if err != nil {
			return "", fmt.Errorf("merge: copy chunk %d: %w", i, err)
		}
	}
	out.Close()
	os.RemoveAll(tempDir) // 清理临时分片

	fileInfo := &domainfile.FileInfo{
		ID:             uuid.NewString(),
		Name:           precheckReq.FileName,
		RandomName:     destName,
		Size:           precheckReq.FileSize,
		Mime:           "application/octet-stream", // 合并后可异步检测
		Path:           destPath,
		ChunkSignature: precheckReq.ChunkSignature,
		IsEnc:          req.IsEnc,
		IsChunk:        true,
		ChunkCount:     totalChunks,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	if len(precheckReq.FilesMd5) > 0 {
		fileInfo.FirstChunkHash = precheckReq.FilesMd5[0]
	}
	if len(precheckReq.FilesMd5) > 1 {
		fileInfo.SecondChunkHash = precheckReq.FilesMd5[1]
	}
	if len(precheckReq.FilesMd5) > 2 {
		fileInfo.ThirdChunkHash = precheckReq.FilesMd5[2]
	}

	if err := uc.deps.FileInfo.Create(ctx, fileInfo); err != nil {
		return "", fmt.Errorf("merge: create file_info: %w", err)
	}

	uf := &domainfile.UserFile{
		ID:          uuid.NewString(),
		UserID:      userID,
		FileID:      fileInfo.ID,
		FileName:    precheckReq.FileName,
		VirtualPath: precheckReq.PathID,
		CreatedAt:   time.Now(),
	}
	if err := uc.deps.UserFile.Create(ctx, uf); err != nil {
		return "", fmt.Errorf("merge: create user_file: %w", err)
	}

	return uf.ID, nil
}

// ── 文件列表 ──────────────────────────────────────────────────

func (uc *UseCase) ListFiles(ctx context.Context, userID string, req FileListReq) (*FileListResp, error) {
	// 解析目录 ID
	var pathID int
	var currentDir *domainfile.VirtualPath
	var err error

	if req.VirtualPath == "" || req.VirtualPath == "0" {
		currentDir, err = uc.deps.VirtualPath.GetRoot(ctx, userID)
	} else {
		pathID, err = strconv.Atoi(req.VirtualPath)
		if err != nil {
			return nil, fmt.Errorf("list: invalid path id: %w", err)
		}
		currentDir, err = uc.deps.VirtualPath.GetByID(ctx, pathID)
	}
	if err != nil {
		return nil, domainfile.ErrDirNotFound
	}
	pathID = currentDir.ID
	pathIDStr := strconv.Itoa(pathID)

	folderCount, _ := uc.deps.VirtualPath.CountSubFolders(ctx, userID, pathID)
	fileCount, _ := uc.deps.UserFile.CountByVirtualPath(ctx, userID, pathIDStr)
	total := folderCount + fileCount

	offset := (req.Page - 1) * req.PageSize
	resp := &FileListResp{
		Breadcrumbs: uc.buildBreadcrumbs(ctx, currentDir),
		CurrentPath: pathIDStr,
		Folders:     []FolderItem{},
		Files:       []FileItem{},
		Total:       total,
		Page:        req.Page,
		PageSize:    req.PageSize,
	}

	// 优先返回目录
	if int64(offset) < folderCount {
		limit := req.PageSize
		if int64(offset+limit) > folderCount {
			limit = int(folderCount) - offset
		}
		folders, _ := uc.deps.VirtualPath.ListSubFolders(ctx, userID, pathID, offset, limit)
		for _, f := range folders {
			resp.Folders = append(resp.Folders, FolderItem{
				ID:          f.ID,
				Name:        f.Path,
				Path:        strconv.Itoa(f.ID),
				CreatedTime: f.CreatedAt,
			})
		}
		// 剩余空间填充文件
		if remaining := req.PageSize - len(folders); remaining > 0 {
			uc.fillFiles(ctx, userID, pathIDStr, 0, remaining, resp)
		}
	} else {
		fileOffset := offset - int(folderCount)
		uc.fillFiles(ctx, userID, pathIDStr, fileOffset, req.PageSize, resp)
	}

	return resp, nil
}

func (uc *UseCase) fillFiles(ctx context.Context, userID, pathIDStr string, offset, limit int, resp *FileListResp) {
	ufs, err := uc.deps.UserFile.ListByVirtualPath(ctx, userID, pathIDStr, offset, limit)
	if err != nil {
		return
	}
	for _, uf := range ufs {
		fi, err := uc.deps.FileInfo.GetByID(ctx, uf.FileID)
		if err != nil {
			continue
		}
		resp.Files = append(resp.Files, FileItem{
			FileID:       uf.ID,
			FileName:     uf.FileName,
			FileSize:     fi.Size,
			MimeType:     fi.Mime,
			IsEnc:        fi.IsEnc,
			HasThumbnail: fi.ThumbnailImg != "",
			IsPublic:     uf.IsPublic,
			CreatedAt:    uf.CreatedAt,
		})
	}
}

func (uc *UseCase) buildBreadcrumbs(ctx context.Context, dir *domainfile.VirtualPath) []BreadcrumbItem {
	var crumbs []BreadcrumbItem
	crumbs = append(crumbs, BreadcrumbItem{
		ID: dir.ID, Name: dir.Path, Path: strconv.Itoa(dir.ID),
	})
	// 最多向上两级
	current := dir
	for i := 0; i < 2 && current.ParentLevel != ""; i++ {
		parentID, err := strconv.Atoi(current.ParentLevel)
		if err != nil {
			break
		}
		parent, err := uc.deps.VirtualPath.GetByID(ctx, parentID)
		if err != nil {
			break
		}
		crumbs = append([]BreadcrumbItem{{
			ID: parent.ID, Name: parent.Path, Path: strconv.Itoa(parent.ID),
		}}, crumbs...)
		current = parent
	}
	return crumbs
}

// ── 目录操作 ──────────────────────────────────────────────────

func (uc *UseCase) MakeDir(ctx context.Context, userID string, req MakeDirReq) error {
	// 检查同名
	existing, err := uc.deps.VirtualPath.GetByPath(ctx, userID, req.DirPath)
	if err == nil && existing != nil {
		return domainfile.ErrDirAlreadyExists
	}

	vp := &domainfile.VirtualPath{
		UserID:    userID,
		Path:      req.DirPath,
		IsDir:     true,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if req.ParentLevel != "" && req.ParentLevel != "0" {
		parentID, err := strconv.Atoi(req.ParentLevel)
		if err != nil {
			return fmt.Errorf("mkdir: invalid parent_level: %w", err)
		}
		parent, err := uc.deps.VirtualPath.GetByID(ctx, parentID)
		if err != nil {
			return domainfile.ErrDirNotFound
		}
		vp.ParentLevel = strconv.Itoa(parent.ID)
	}
	return uc.deps.VirtualPath.Create(ctx, vp)
}

func (uc *UseCase) RenameDir(ctx context.Context, userID string, req RenameDirReq) error {
	dir, err := uc.deps.VirtualPath.GetByID(ctx, req.DirID)
	if err != nil {
		return domainfile.ErrDirNotFound
	}
	if dir.UserID != userID {
		return domainfile.ErrForbidden
	}
	root, err := uc.deps.VirtualPath.GetRoot(ctx, userID)
	if err == nil && root.ID == req.DirID {
		return domainfile.ErrDirIsRoot
	}

	newPath := "/" + strings.TrimSpace(req.NewDirName)

	// 同级重名检查
	siblings, _ := uc.deps.VirtualPath.ListSubFolders(ctx, userID,
		func() int {
			if dir.ParentLevel == "" {
				if root != nil {
					return root.ID
				}
				return 0
			}
			id, _ := strconv.Atoi(dir.ParentLevel)
			return id
		}(),
		0, 1000,
	)
	for _, s := range siblings {
		if s.Path == newPath && s.ID != req.DirID {
			return domainfile.ErrDirNameConflict
		}
	}

	dir.Path = newPath
	dir.UpdatedAt = time.Now()
	return uc.deps.VirtualPath.Update(ctx, dir)
}

func (uc *UseCase) DeleteDir(ctx context.Context, userID string, req DeleteDirReq) error {
	dir, err := uc.deps.VirtualPath.GetByID(ctx, req.DirID)
	if err != nil {
		return domainfile.ErrDirNotFound
	}
	if dir.UserID != userID {
		return domainfile.ErrForbidden
	}
	root, _ := uc.deps.VirtualPath.GetRoot(ctx, userID)
	if root != nil && root.ID == req.DirID {
		return domainfile.ErrDirIsRoot
	}

	// 收集所有子目录并递归删除
	var collectDirs func(parentID int) []int
	collectDirs = func(parentID int) []int {
		subs, _ := uc.deps.VirtualPath.ListSubFolders(ctx, userID, parentID, 0, 10000)
		ids := make([]int, 0, len(subs))
		for _, s := range subs {
			ids = append(ids, s.ID)
			ids = append(ids, collectDirs(s.ID)...)
		}
		return ids
	}

	allDirs := append([]int{req.DirID}, collectDirs(req.DirID)...)
	for _, id := range allDirs {
		_ = uc.deps.VirtualPath.Delete(ctx, id)
	}
	return nil
}

// ── 文件操作 ──────────────────────────────────────────────────

func (uc *UseCase) DeleteFiles(ctx context.Context, userID string, req DeleteFilesReq) (*DeleteFilesResp, error) {
	resp := &DeleteFilesResp{}
	for _, ufID := range req.FileIDs {
		// 验证归属权
		if _, err := uc.deps.UserFile.GetByUserAndUfID(ctx, userID, ufID); err != nil {
			resp.Failed++
			resp.Errors = append(resp.Errors, fmt.Sprintf("%s: not found", ufID))
			continue
		}
		// 软删除 + 创建回收站记录（由注入的函数完成，保持域边界）
		if err := uc.deps.MoveToRecycled(ctx, userID, ufID); err != nil {
			resp.Failed++
			resp.Errors = append(resp.Errors, fmt.Sprintf("%s: %v", ufID, err))
			continue
		}
		resp.Success++
	}
	return resp, nil
}

func (uc *UseCase) MoveFile(ctx context.Context, userID string, req MoveFileReq) error {
	uf, err := uc.deps.UserFile.GetByUserAndUfID(ctx, userID, req.FileID)
	if err != nil {
		return domainfile.ErrNotFound
	}
	uf.VirtualPath = req.TargetPath
	return uc.deps.UserFile.Update(ctx, uf)
}

func (uc *UseCase) RenameFile(ctx context.Context, userID string, req RenameFileReq) error {
	newName := strings.TrimSpace(req.NewFileName)
	if newName == "" {
		return fmt.Errorf("rename: name is empty")
	}
	uf, err := uc.deps.UserFile.GetByUserAndUfID(ctx, userID, req.FileID)
	if err != nil {
		return domainfile.ErrNotFound
	}

	// 同目录重名检查
	siblings, _ := uc.deps.UserFile.ListByVirtualPath(ctx, userID, uf.VirtualPath, 0, 10000)
	for _, s := range siblings {
		if s.FileName == newName && s.ID != uf.ID {
			return domainfile.ErrFileNameConflict
		}
	}

	uf.FileName = newName
	return uc.deps.UserFile.Update(ctx, uf)
}

func (uc *UseCase) SetPublic(ctx context.Context, userID string, req SetPublicReq) error {
	uf, err := uc.deps.UserFile.GetByUserAndUfID(ctx, userID, req.FileID)
	if err != nil {
		return domainfile.ErrNotFound
	}
	if req.Public {
		fi, err := uc.deps.FileInfo.GetByID(ctx, uf.FileID)
		if err == nil && fi.IsEnc {
			return domainfile.ErrEncryptedPublic
		}
	}
	uf.IsPublic = req.Public
	return uc.deps.UserFile.Update(ctx, uf)
}

// ── 上传任务管理 ──────────────────────────────────────────────

func (uc *UseCase) GetUploadProgress(ctx context.Context, userID string, req UploadProgressReq) (*UploadProgressResp, error) {
	task, err := uc.deps.UploadTask.GetByID(ctx, req.PrecheckID)
	if err != nil {
		return nil, domainfile.ErrTaskNotFound
	}
	if task.UserID != userID {
		return nil, domainfile.ErrForbidden
	}
	return &UploadProgressResp{
		PrecheckID: task.ID,
		FileName:   task.FileName,
		FileSize:   task.FileSize,
		Uploaded:   task.UploadedChunks,
		Total:      task.TotalChunks,
		Progress:   task.Progress(),
		IsComplete: task.IsComplete(),
	}, nil
}

func (uc *UseCase) ListUploadTasks(ctx context.Context, userID string, req TaskListReq) (*TaskListResp, error) {
	offset := (req.Page - 1) * req.PageSize
	tasks, err := uc.deps.UploadTask.ListByUser(ctx, userID, offset, req.PageSize)
	if err != nil {
		return nil, err
	}
	total, _ := uc.deps.UploadTask.CountByUser(ctx, userID)

	items := make([]TaskItem, 0, len(tasks))
	for _, t := range tasks {
		items = append(items, toTaskItem(t))
	}
	return &TaskListResp{Tasks: items, Total: total, Page: req.Page, PageSize: req.PageSize}, nil
}

func (uc *UseCase) DeleteUploadTask(ctx context.Context, userID, taskID string) error {
	task, err := uc.deps.UploadTask.GetByID(ctx, taskID)
	if err != nil {
		return domainfile.ErrTaskNotFound
	}
	if task.UserID != userID {
		return domainfile.ErrForbidden
	}
	return uc.deps.UploadTask.Delete(ctx, taskID)
}

func (uc *UseCase) RenewUploadTask(ctx context.Context, userID string, req RenewTaskReq) (*TaskItem, error) {
	task, err := uc.deps.UploadTask.GetByID(ctx, req.TaskID)
	if err != nil {
		return nil, domainfile.ErrTaskNotFound
	}
	if task.UserID != userID {
		return nil, domainfile.ErrForbidden
	}
	if !task.IsExpired() {
		return nil, domainfile.ErrTaskNotExpired
	}
	days := req.Days
	if days <= 0 {
		days = 7
	}
	task.ExpiresAt = time.Now().Add(time.Duration(days) * 24 * time.Hour)
	task.UpdatedAt = time.Now()
	if err := uc.deps.UploadTask.Update(ctx, task); err != nil {
		return nil, err
	}
	item := toTaskItem(task)
	return &item, nil
}

// ── 内部辅助 ──────────────────────────────────────────────────

func (uc *UseCase) updateTaskProgress(ctx context.Context, id string, uploaded, total int, tempDir string, status domainfile.UploadStatus, errMsg string) error {
	task, err := uc.deps.UploadTask.GetByID(ctx, id)
	if err != nil {
		return err
	}
	task.UploadedChunks = uploaded
	task.Status = status
	task.ErrorMessage = errMsg
	if tempDir != "" {
		task.TempDir = tempDir
	}
	task.UpdatedAt = time.Now()
	return uc.deps.UploadTask.Update(ctx, task)
}

func (uc *UseCase) cleanPrecheckCache(precheckID string) {
	_ = uc.deps.Cache.Delete(precheckCacheKey(precheckID))
}

func (uc *UseCase) cacheSet(key string, v any, ttlSec int) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return uc.deps.Cache.Set(key, string(b), ttlSec)
}

func (uc *UseCase) cacheGet(key string, dst any) error {
	raw, err := uc.deps.Cache.Get(key)
	if err != nil {
		return err
	}
	return json.Unmarshal([]byte(raw.(string)), dst)
}

func precheckCacheKey(id string) string { return "precheck:" + id }

func countChunks(dir string, total int) int {
	n := 0
	for i := 0; i < total; i++ {
		if _, err := os.Stat(filepath.Join(dir, fmt.Sprintf("%d.chunk", i))); err == nil {
			n++
		}
	}
	return n
}

func saveToFile(r io.Reader, path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, r)
	return err
}

// sanitize 去掉文件名中不适合作目录名的字符。
func sanitize(name string) string {
	r := strings.NewReplacer("/", "_", "\\", "_", ":", "_", " ", "_")
	s := r.Replace(name)
	if len(s) > 40 {
		s = s[:40]
	}
	return s
}

func toTaskItem(t *domainfile.UploadTask) TaskItem {
	return TaskItem{
		ID:             t.ID,
		FileName:       t.FileName,
		FileSize:       t.FileSize,
		ChunkSize:      t.ChunkSize,
		TotalChunks:    t.TotalChunks,
		UploadedChunks: t.UploadedChunks,
		Progress:       t.Progress(),
		Status:         string(t.Status),
		ErrorMessage:   t.ErrorMessage,
		PathID:         t.PathID,
		CreatedAt:      t.CreatedAt,
		UpdatedAt:      t.UpdatedAt,
		ExpiresAt:      t.ExpiresAt,
	}
}
