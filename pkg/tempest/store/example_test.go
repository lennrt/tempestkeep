package store_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/lennrt/tempestkeep/pkg/tempest/model"
	"github.com/lennrt/tempestkeep/pkg/tempest/store"
)

func ExampleOpenWriter() {
	dir, err := os.MkdirTemp("", "tempestkeep-store-example-")
	if err != nil {
		panic(err)
	}
	path := filepath.Join(dir, "archive.sqlite")
	defer removeExampleArchive(dir, path)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	writer, err := store.OpenWriter(ctx, path)
	if err != nil {
		panic(err)
	}
	writerClosed := false
	defer func() {
		if !writerClosed {
			if err := writer.Close(); err != nil {
				panic(err)
			}
		}
	}()
	added, err := writer.InsertObs(ctx, 7, []model.DeviceObs{{Epoch: 1}})
	if err != nil {
		panic(err)
	}
	if err := writer.Close(); err != nil {
		panic(err)
	}
	writerClosed = true

	archive, err := store.Open(ctx, path)
	if err != nil {
		panic(err)
	}
	archiveClosed := false
	defer func() {
		if !archiveClosed {
			if err := archive.Close(); err != nil {
				panic(err)
			}
		}
	}()
	latest, err := archive.Latest(ctx)
	if err != nil {
		panic(err)
	}
	if err := archive.Close(); err != nil {
		panic(err)
	}
	archiveClosed = true
	fmt.Println(added, latest.Epoch)
	// Output: 1 1
}

func removeExampleArchive(dir, path string) {
	for _, candidate := range []string{path, path + "-wal", path + "-shm", path + "-journal"} {
		if err := os.Remove(candidate); err != nil && !os.IsNotExist(err) {
			panic(err)
		}
	}
	if err := os.Remove(dir); err != nil {
		panic(err)
	}
}
