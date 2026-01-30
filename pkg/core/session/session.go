package session

import (
	"context"
	"errors"
)

var ErrKeyNotFound = errors.New("key not found")

type Session interface {
	Get(ctx context.Context, key string) (any, error)
	Set(ctx context.Context, key string, value any) error
	Delete(ctx context.Context, key string) error
}

type SessionManager interface {
	Get(ctx context.Context, chatID int64) (Session, error)
}
