package gormrepo

import (
	"context"

	"gorm.io/gorm"
)

type txKeyType struct{}

var txKey = txKeyType{}

func withTx(ctx context.Context, tx *gorm.DB) context.Context {
	return context.WithValue(ctx, txKey, tx)
}

func getDBFromCtx(ctx context.Context, base *gorm.DB) *gorm.DB {
	if v := ctx.Value(txKey); v != nil {
		if tx, ok := v.(*gorm.DB); ok && tx != nil {
			return tx
		}
	}
	return base
}
