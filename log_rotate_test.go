package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDailyLogWriterWritesAndCleansOldFiles(t *testing.T) {
	dir := t.TempDir()
	oldDate := time.Now().AddDate(0, 0, -3).Format("2006-01-02")
	oldPath := filepath.Join(dir, logFilePrefix+oldDate+".log")
	if err := os.WriteFile(oldPath, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	writer, err := NewDailyLogWriter(dir, 2)
	if err != nil {
		t.Fatalf("NewDailyLogWriter() error = %v", err)
	}
	defer writer.Close()

	if _, err := writer.Write([]byte("hello\n")); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	todayPath := filepath.Join(dir, logFilePrefix+time.Now().Format("2006-01-02")+".log")
	if _, err := os.Stat(todayPath); err != nil {
		t.Fatalf("today log file missing: %v", err)
	}
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatalf("old log file was not removed, stat err = %v", err)
	}
}
