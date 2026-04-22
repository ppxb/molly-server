package user

import (
	"errors"
)

// domain 层只定义业务语义错误，不关心 HTTP 状态码。
// 状态码映射由 presentation 层负责。
var (
	ErrNotFound          = errors.New("user: not found")
	ErrInvalidCredential = errors.New("user: invalid username or password")
	ErrDisabled          = errors.New("user: account is disabled")
	ErrUserNameConflict  = errors.New("user: username already exists")
	ErrEmailConflict     = errors.New("user: email already exists")
	ErrInsufficientSpace = errors.New("user: insufficient storage space")
)
