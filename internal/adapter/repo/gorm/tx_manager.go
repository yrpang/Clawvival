package gormrepo

import (
	"context"

	"gorm.io/gorm"
)

type TxManager struct {
	db *gorm.DB
}

func NewTxManager(db *gorm.DB) TxManager {
	return TxManager{db: db}
}

func (t TxManager) RunInTx(ctx context.Context, fn func(ctx context.Context) error) error {
	return t.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(withTx(ctx, tx))
	})
}
