package autoupdate

import "errors"

var (
	ErrAlreadyExists = errors.New("already exist")
	ErrNotFound      = errors.New("not found")
)
