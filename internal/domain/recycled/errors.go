package recycled

import "errors"

var (
	ErrNotFound  = errors.New("recycled: record not found")
	ErrForbidden = errors.New("recycled: access denied")
)
