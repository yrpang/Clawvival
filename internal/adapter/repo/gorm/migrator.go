package gormrepo

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"
)

func ApplyMigrations(ctx context.Context, db *gorm.DB, dir string) error {
	createMetaTableSQL := `
CREATE TABLE IF NOT EXISTS schema_migrations (
  version TEXT PRIMARY KEY,
  applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);`
	if err := db.WithContext(ctx).Exec(createMetaTableSQL).Error; err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read migration dir: %w", err)
	}

	files := make([]string, 0)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasSuffix(name, ".sql") {
			files = append(files, name)
		}
	}
	sort.Strings(files)

	for _, name := range files {
		version := strings.TrimSuffix(name, ".sql")
		var count int64
		if err := db.WithContext(ctx).Table("schema_migrations").Where("version = ?", version).Count(&count).Error; err != nil {
			return fmt.Errorf("check migration %s: %w", version, err)
		}
		if count > 0 {
			continue
		}

		content, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return fmt.Errorf("read migration %s: %w", name, err)
		}

		err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := tx.Exec(string(content)).Error; err != nil {
				return fmt.Errorf("apply migration %s: %w", name, err)
			}
			if err := tx.Exec(`INSERT INTO schema_migrations(version, applied_at) VALUES (?, ?)`, version, time.Now()).Error; err != nil {
				return fmt.Errorf("record migration %s: %w", version, err)
			}
			return nil
		})
		if err != nil {
			return err
		}
	}

	return nil
}
