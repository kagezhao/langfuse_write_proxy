package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const logFilePrefix = "langfuse-write-proxy-"

type DailyLogWriter struct {
	mu          sync.Mutex
	dir         string
	maxDays     int
	currentDate string
	file        *os.File
}

func NewDailyLogWriter(dir string, maxDays int) (*DailyLogWriter, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create log dir: %w", err)
	}
	return &DailyLogWriter{dir: dir, maxDays: maxDays}, nil
}

func (w *DailyLogWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	today := time.Now().Format("2006-01-02")
	if w.file == nil || w.currentDate != today {
		if err := w.rotate(today); err != nil {
			return 0, err
		}
	}

	return w.file.Write(p)
}

func (w *DailyLogWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.file == nil {
		return nil
	}
	return w.file.Close()
}

func (w *DailyLogWriter) rotate(date string) error {
	if w.file != nil {
		_ = w.file.Close()
		w.file = nil
	}

	path := filepath.Join(w.dir, logFilePrefix+date+".log")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open log file: %w", err)
	}

	w.file = file
	w.currentDate = date
	w.cleanup()
	return nil
}

func (w *DailyLogWriter) cleanup() {
	if w.maxDays <= 0 {
		return
	}

	cutoff := time.Now().AddDate(0, 0, -w.maxDays+1)
	files, err := filepath.Glob(filepath.Join(w.dir, logFilePrefix+"*.log"))
	if err != nil {
		return
	}

	for _, path := range files {
		name := filepath.Base(path)
		dateText := strings.TrimSuffix(strings.TrimPrefix(name, logFilePrefix), ".log")
		date, err := time.Parse("2006-01-02", dateText)
		if err != nil {
			continue
		}
		if date.Before(dateOnly(cutoff)) {
			_ = os.Remove(path)
		}
	}
}

func dateOnly(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}
