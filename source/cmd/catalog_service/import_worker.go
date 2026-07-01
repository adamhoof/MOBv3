package main

import (
	"context"
	"fmt"
	"os"

	"github.com/adamhoof/MOBv3/internal/catalog"
)

type importRunner struct {
	store catalog.Store
	sem   chan struct{}
}

type importResult struct {
	imported int64
	status   int
	err      error
}

func newImportRunner(store catalog.Store) *importRunner {
	return &importRunner{store: store, sem: make(chan struct{}, 1)}
}

func (r *importRunner) run(ctx context.Context, gzPath string) (importResult, bool) {
	select {
	case r.sem <- struct{}{}:
	case <-ctx.Done():
		return importResult{err: ctx.Err()}, true
	default:
		return importResult{}, false
	}

	resultCh := make(chan importResult, 1)
	go func() {
		defer func() { <-r.sem }()
		resultCh <- processImport(ctx, r.store, gzPath)
	}()

	select {
	case result := <-resultCh:
		return result, true
	case <-ctx.Done():
		return importResult{err: ctx.Err()}, true
	}
}

func processImport(ctx context.Context, store catalog.Store, gzPath string) importResult {
	defer os.Remove(gzPath)

	mdbPath, err := decompressGzip(gzPath)
	if err != nil {
		return importResult{status: 400, err: fmt.Errorf("decompress upload: %w", err)}
	}
	defer os.Remove(mdbPath)

	csvPath, err := convertMDBToCSV(mdbPath)
	if err != nil {
		return importResult{status: 422, err: fmt.Errorf("convert upload: %w", err)}
	}
	defer os.Remove(csvPath)

	csvFile, err := os.Open(csvPath)
	if err != nil {
		return importResult{status: 500, err: fmt.Errorf("open converted csv: %w", err)}
	}
	defer csvFile.Close()

	products, parseErrors := catalog.ParseCSV(csvFile)
	imported, err := store.Import(ctx, products, parseErrors)
	if err != nil {
		return importResult{imported: imported, status: 500, err: err}
	}
	return importResult{imported: imported}
}
