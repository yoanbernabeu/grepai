package framework

import "errors"

var (
	ErrNotImplemented = errors.New("framework processor not implemented")
	ErrUnavailable    = errors.New("framework processor unavailable")
)
