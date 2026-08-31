package collect_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/lennrt/tempestkeep/pkg/tempest/collect"
	"github.com/lennrt/tempestkeep/pkg/tempest/model"
	"github.com/lennrt/tempestkeep/pkg/tempest/store"
)

type exampleFetcher struct{}

func (exampleFetcher) DeviceObservations(_ context.Context, _ int, start, end int64) ([]model.DeviceObs, error) {
	if start < 0 || end < start || end-start > 2 {
		return nil, fmt.Errorf("example range must contain at most three seconds")
	}
	observations := make([]model.DeviceObs, 0, int(end-start+1))
	for epoch := range end - start + 1 {
		observations = append(observations, model.DeviceObs{Epoch: start + epoch})
	}
	return observations, nil
}

func ExampleBackfiller_BackfillRange() {
	dir, err := os.MkdirTemp("", "tempestkeep-collect-example-")
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
	backfiller, err := collect.New(exampleFetcher{}, writer, 7, collect.WithChunkSeconds(2))
	if err != nil {
		panic(err)
	}
	result, err := backfiller.BackfillRange(ctx, 10, 12, 2)
	if err != nil {
		panic(err)
	}
	if err := writer.Close(); err != nil {
		panic(err)
	}
	writerClosed = true
	fmt.Println(result.Fetched, result.RowsAdded, result.Resume, result.Done)
	// Output: 3 3 13 true
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
