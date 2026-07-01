package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

type importResponse struct {
	Status   string `json:"status"`
	Imported int64  `json:"imported,omitempty"`
	Duration string `json:"duration,omitempty"`
	Error    string `json:"error,omitempty"`
}

func handleImport(runner *importRunner, cfg serviceConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		started := time.Now()
		r.Body = http.MaxBytesReader(w, r.Body, cfg.MaxUploadSize)
		defer r.Body.Close()

		gzPath, err := saveUpload(r.Body)
		if err != nil {
			writeImportError(w, http.StatusBadRequest, err)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), cfg.ImportTimeout)
		defer cancel()

		result, accepted := runner.run(ctx, gzPath)
		if !accepted {
			os.Remove(gzPath)
			writeImportError(w, http.StatusConflict, fmt.Errorf("catalog import already running"))
			return
		}
		if result.err != nil {
			status := result.status
			if status == 0 {
				status = http.StatusInternalServerError
			}
			writeImportError(w, status, result.err)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(importResponse{
			Status:   "completed",
			Imported: result.imported,
			Duration: time.Since(started).Round(time.Millisecond).String(),
		})
	}
}

func saveUpload(r io.Reader) (string, error) {
	file, err := os.CreateTemp("", "catalog_*.mdb.gz")
	if err != nil {
		return "", fmt.Errorf("create upload temp file: %w", err)
	}
	defer file.Close()

	if _, err := io.Copy(file, r); err != nil {
		os.Remove(file.Name())
		return "", fmt.Errorf("save upload: %w", err)
	}
	return file.Name(), nil
}

func convertMDBToCSV(mdbPath string) (string, error) {
	output, err := os.CreateTemp("", "catalog_*.csv")
	if err != nil {
		return "", fmt.Errorf("create csv temp file: %w", err)
	}
	csvPath := output.Name()
	if err := output.Close(); err != nil {
		os.Remove(csvPath)
		return "", fmt.Errorf("close csv temp file: %w", err)
	}

	cmd := exec.Command(mdbConverterPath(), mdbPath, csvPath)
	outputBytes, err := cmd.CombinedOutput()
	if err != nil {
		os.Remove(csvPath)
		return "", fmt.Errorf("convert mdb: %w: %s", err, commandOutput(outputBytes))
	}
	return csvPath, nil
}

func mdbConverterPath() string {
	if path := strings.TrimSpace(os.Getenv("MDB_CONVERTER_PATH")); path != "" {
		return path
	}
	return "./mdb_to_csv.sh"
}

func commandOutput(output []byte) string {
	text := strings.TrimSpace(string(output))
	if text == "" {
		return "no command output"
	}
	if len(text) > 2000 {
		return text[:2000] + "..."
	}
	return text
}

func writeImportError(w http.ResponseWriter, status int, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(importResponse{Status: "failed", Error: err.Error()})
}
