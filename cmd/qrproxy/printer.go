package main

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func runPrinter(ctx context.Context, cfg config, ch <-chan []byte) {
	for {
		select {
		case <-ctx.Done():
			return
		case data := <-ch:
			relayToPrinter(cfg.USBPrinter, data)
		}
	}
}

func resolveDevice(pattern string) string {
	if !strings.ContainsAny(pattern, "*?[") {
		return pattern
	}
	matches, err := filepath.Glob(pattern)
	if err != nil || len(matches) == 0 {
		slog.Warn("no printer matching", "pattern", pattern)
		return ""
	}
	sort.Strings(matches)
	slog.Debug("printer resolved", "device", matches[0])
	return matches[0]
}

func relayToPrinter(pattern string, data []byte) {
	device := resolveDevice(pattern)
	if device == "" {
		return
	}
	f, err := os.OpenFile(device, os.O_WRONLY, 0)
	if err != nil {
		slog.Error("printer open error", "err", err)
		return
	}
	defer f.Close()
	if _, err := f.Write(data); err != nil {
		slog.Error("printer write error", "err", err)
		return
	}
	time.Sleep(500 * time.Millisecond)
}
