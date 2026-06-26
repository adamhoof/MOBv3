package main

import (
	"compress/gzip"
	"fmt"
	"io"
	"os"
)

func decompressGzip(path string) (string, error) {
	input, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open gzip upload: %w", err)
	}
	defer input.Close()

	gzipReader, err := gzip.NewReader(input)
	if err != nil {
		return "", fmt.Errorf("read gzip upload: %w", err)
	}
	defer gzipReader.Close()

	output, err := os.CreateTemp("", "catalog_*.mdb")
	if err != nil {
		return "", fmt.Errorf("create mdb temp file: %w", err)
	}
	defer output.Close()

	if _, err := io.Copy(output, gzipReader); err != nil {
		os.Remove(output.Name())
		return "", fmt.Errorf("decompress upload: %w", err)
	}
	return output.Name(), nil
}
