package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"gorm.io/driver/postgres"
	"gorm.io/gen"
	"gorm.io/gorm"
)

func main() {
	var dsn, out string
	flag.StringVar(&dsn, "dsn", os.Getenv("CLAWVERSE_DB_DSN"), "postgres dsn")
	flag.StringVar(&out, "out", "internal/adapter/repo/gorm/model", "output dir for generated models")
	flag.Parse()

	if dsn == "" {
		log.Fatal("missing --dsn or CLAWVERSE_DB_DSN")
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("open postgres: %v", err)
	}

	g := gen.NewGenerator(gen.Config{
		OutPath:      out,
		ModelPkgPath: "model",
		Mode:         gen.WithoutContext | gen.WithDefaultQuery,
	})
	g.UseDB(db)
	g.GenerateAllTable()
	g.Execute()

	fmt.Printf("generated gorm models at %s\n", out)
}
