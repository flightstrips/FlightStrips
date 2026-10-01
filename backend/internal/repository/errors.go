package repository

import "errors"

// ErrNotFound identifies an absent entity in a detached or accepted-state read.
var ErrNotFound = errors.New("entity not found")
