package file

import "errors"

var (
	ErrNotFound          = errors.New("file: not found")
	ErrDirNotFound       = errors.New("file: directory not found")
	ErrDirAlreadyExists  = errors.New("file: directory already exists")
	ErrDirIsRoot         = errors.New("file: root directory cannot be modified or deleted")
	ErrDirNameConflict   = errors.New("file: directory name already exists at this level")
	ErrFileNameConflict  = errors.New("file: file name already exists in this directory")
	ErrEncryptedPublic   = errors.New("file: encrypted file cannot be set as public")
	ErrInsufficientSpace = errors.New("file: insufficient storage space")
	ErrNoDiskAvailable   = errors.New("file: no disk with sufficient space available")
	ErrPrecheckExpired   = errors.New("file: precheck session expired or not found")
	ErrForbidden         = errors.New("file: access denied")
	ErrTaskNotFound      = errors.New("file: upload task not found")
	ErrTaskNotExpired    = errors.New("file: upload task has not expired yet")
	ErrAlreadyInRecycled = errors.New("file: already in recycle bin")
)
