package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

func TestRunGenerateRequestsAndSavesExactlyOneImage(t *testing.T) {
	const png = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII="
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path != "/v1/images/generations" {
			t.Errorf("request path = %q, want /v1/images/generations", r.URL.Path)
		}
		var payload struct {
			N int `json:"n"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if payload.N != 1 {
			t.Errorf("request n = %d, want 1", payload.N)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"data":[{"b64_json":%q}]}`, png)
	}))
	defer server.Close()

	t.Setenv("IMAGE_API_KEY", "test-key")
	t.Setenv("IMAGE_API_MAX_ATTEMPTS", "1")
	outDir := t.TempDir()
	var output bytes.Buffer
	if err := runGenerate([]string{
		"--prompt", "a test image",
		"--base-url", server.URL,
		"--out-dir", outDir,
		"--max-attempts", "1",
	}, &output); err != nil {
		t.Fatalf("runGenerate returned an error: %v", err)
	}
	if requests != 1 {
		t.Fatalf("server received %d requests, want 1", requests)
	}
	var result struct {
		Outputs []string `json:"outputs"`
	}
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatalf("decode command output: %v", err)
	}
	if len(result.Outputs) != 1 {
		t.Fatalf("command returned %d outputs, want 1", len(result.Outputs))
	}
	if _, err := os.Stat(result.Outputs[0]); err != nil {
		t.Fatalf("returned output %q is not saved: %v", result.Outputs[0], err)
	}
}

func TestRunGenerateRejectsPublicImageCount(t *testing.T) {
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
	}))
	defer server.Close()

	t.Setenv("IMAGE_API_KEY", "test-key")
	err := runGenerate([]string{
		"--prompt", "a test image",
		"--base-url", server.URL,
		"--n", "2",
	}, io.Discard)
	if err == nil {
		t.Fatal("runGenerate accepted the public --n option")
	}
	if !strings.Contains(err.Error(), "flag provided but not defined: -n") {
		t.Fatalf("runGenerate error = %q, want unknown --n option", err)
	}
	if requests != 0 {
		t.Fatalf("server received %d requests after rejected arguments, want 0", requests)
	}
}

func TestRunEditRequestsAndSavesExactlyOneImage(t *testing.T) {
	const png = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII="
	inputDir := t.TempDir()
	input := filepath.Join(inputDir, "input.png")
	if err := os.WriteFile(input, []byte("input image"), 0644); err != nil {
		t.Fatal(err)
	}
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path != "/v1/images/edits" {
			t.Errorf("request path = %q, want /v1/images/edits", r.URL.Path)
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Errorf("parse multipart request: %v", err)
		} else if got := r.FormValue("n"); got != "1" {
			t.Errorf("request n = %q, want 1", got)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"data":[{"b64_json":%q}]}`, png)
	}))
	defer server.Close()

	t.Setenv("IMAGE_API_KEY", "test-key")
	t.Setenv("IMAGE_API_MAX_ATTEMPTS", "1")
	outDir := t.TempDir()
	var output bytes.Buffer
	if err := runEdit([]string{
		"--image", input,
		"--prompt", "edit the image",
		"--base-url", server.URL,
		"--out-dir", outDir,
		"--max-attempts", "1",
	}, &output); err != nil {
		t.Fatalf("runEdit returned an error: %v", err)
	}
	if requests != 1 {
		t.Fatalf("server received %d requests, want 1", requests)
	}
	var result struct {
		Outputs []string `json:"outputs"`
	}
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatalf("decode command output: %v", err)
	}
	if len(result.Outputs) != 1 {
		t.Fatalf("command returned %d outputs, want 1", len(result.Outputs))
	}
	if _, err := os.Stat(result.Outputs[0]); err != nil {
		t.Fatalf("returned output %q is not saved: %v", result.Outputs[0], err)
	}
}

func TestRunEditRejectsPublicImageCount(t *testing.T) {
	input := filepath.Join(t.TempDir(), "input.png")
	if err := os.WriteFile(input, []byte("input image"), 0644); err != nil {
		t.Fatal(err)
	}
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
	}))
	defer server.Close()

	err := runEdit([]string{
		"--image", input,
		"--prompt", "edit the image",
		"--base-url", server.URL,
		"--n", "2",
	}, io.Discard)
	if err == nil {
		t.Fatal("runEdit accepted the public --n option")
	}
	if !strings.Contains(err.Error(), "flag provided but not defined: -n") {
		t.Fatalf("runEdit error = %q, want unknown --n option", err)
	}
	if requests != 0 {
		t.Fatalf("server received %d requests after rejected arguments, want 0", requests)
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

func TestSingleImageCommandsRejectUnexpectedResponseCardinality(t *testing.T) {
	const png = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII="
	for _, count := range []int{0, 2} {
		t.Run(fmt.Sprintf("%d images", count), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if count == 0 {
					fmt.Fprint(w, `{"data":[]}`)
					return
				}
				fmt.Fprintf(w, `{"data":[{"b64_json":%q},{"b64_json":%q}]}`, png, png)
			}))
			defer server.Close()

			t.Setenv("IMAGE_API_KEY", "test-key")
			t.Setenv("IMAGE_API_MAX_ATTEMPTS", "1")
			outDir := t.TempDir()
			var output bytes.Buffer
			err := runGenerate([]string{
				"--prompt", "a test image",
				"--base-url", server.URL,
				"--out-dir", outDir,
				"--max-attempts", "1",
			}, &output)
			if err == nil {
				t.Fatalf("runGenerate succeeded for %d returned images", count)
			}
			if count == 0 && !strings.Contains(err.Error(), "no image data") {
				t.Fatalf("runGenerate error = %q, want no-image failure", err)
			}
			if count == 2 && !strings.Contains(err.Error(), "returned 2 image(s), expected 1") {
				t.Fatalf("runGenerate error = %q, want multiple-image failure", err)
			}
			entries, err := os.ReadDir(outDir)
			if err != nil {
				t.Fatal(err)
			}
			if len(entries) != 0 {
				t.Fatalf("saved %d output files after cardinality failure, want 0", len(entries))
			}
		})
	}
}
