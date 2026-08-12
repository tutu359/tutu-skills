package main

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type failingWriter struct {
	err error
}

func (w failingWriter) Write([]byte) (int, error) {
	return 0, w.err
}

func TestWriteJSON(t *testing.T) {
	var output bytes.Buffer
	if err := writeJSON(&output, map[string]any{"ok": true}); err != nil {
		t.Fatalf("writeJSON returned an error: %v", err)
	}
	if got, want := output.String(), "{\n  \"ok\": true\n}\n"; got != want {
		t.Fatalf("writeJSON output = %q, want %q", got, want)
	}
}

func TestWriteJSONReturnsWriterError(t *testing.T) {
	want := errors.New("write failed")
	err := writeJSON(failingWriter{err: want}, map[string]any{"ok": true})
	if !errors.Is(err, want) {
		t.Fatalf("writeJSON error = %v, want wrapped %v", err, want)
	}
	if !strings.Contains(err.Error(), "could not write JSON output") {
		t.Fatalf("writeJSON error = %q, want output context", err)
	}
}

func TestRunBatchReturnsOutputErrorAfterSavingImage(t *testing.T) {
	const png = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII="
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"data":[{"b64_json":%q}]}`, png)
	}))
	defer server.Close()

	t.Setenv("IMAGE_API_KEY", "test-key")
	t.Setenv("IMAGE_API_MAX_ATTEMPTS", "1")
	t.Setenv("IMAGE_API_BATCH_CONCURRENCY", "1")
	tempDir := t.TempDir()
	input := filepath.Join(tempDir, "jobs.jsonl")
	if err := os.WriteFile(input, []byte("{\"prompt\":\"test image\",\"out\":\"result.png\"}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	outDir := filepath.Join(tempDir, "output")
	want := errors.New("write failed")
	err := runBatch([]string{
		"--input", input,
		"--out-dir", outDir,
		"--concurrency", "1",
		"--base-url", server.URL,
		"--max-attempts", "1",
	}, failingWriter{err: want})
	if !errors.Is(err, want) {
		t.Fatalf("runBatch error = %v, want wrapped %v", err, want)
	}
	if _, err := os.Stat(filepath.Join(outDir, "result.png")); err != nil {
		t.Fatalf("generated image was not saved before the output failure: %v", err)
	}
}
