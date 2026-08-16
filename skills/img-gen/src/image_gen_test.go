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
	"sync"
	"testing"
	"time"
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
	if err := os.WriteFile(input, []byte("{\"operation\":\"generate\",\"prompt\":\"test image\",\"out\":\"result.png\"}\n"), 0644); err != nil {
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

func TestRunEditRejectsUnexpectedResponseCardinality(t *testing.T) {
	const png = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII="
	for _, count := range []int{0, 2} {
		t.Run(fmt.Sprintf("%d images", count), func(t *testing.T) {
			input := filepath.Join(t.TempDir(), "input.png")
			if err := os.WriteFile(input, []byte("input image"), 0644); err != nil {
				t.Fatal(err)
			}
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
			err := runEdit([]string{
				"--image", input,
				"--prompt", "edit the image",
				"--base-url", server.URL,
				"--out-dir", outDir,
				"--max-attempts", "1",
			}, &output)
			if err == nil {
				t.Fatalf("runEdit succeeded for %d returned images", count)
			}
			if count == 0 && !strings.Contains(err.Error(), "no image data") {
				t.Fatalf("runEdit error = %q, want no-image failure", err)
			}
			if count == 2 && !strings.Contains(err.Error(), "returned 2 image(s), expected 1") {
				t.Fatalf("runEdit error = %q, want multiple-image failure", err)
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

func writeBatchInput(t *testing.T, jobs string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "jobs.jsonl")
	if err := os.WriteFile(path, []byte(jobs), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func decodeBatchSummary(t *testing.T, output *bytes.Buffer) struct {
	Concurrency int           `json:"concurrency"`
	Jobs        []batchResult `json:"jobs"`
	Succeeded   int           `json:"succeeded"`
	Failed      int           `json:"failed"`
} {
	t.Helper()
	var summary struct {
		Concurrency int           `json:"concurrency"`
		Jobs        []batchResult `json:"jobs"`
		Succeeded   int           `json:"succeeded"`
		Failed      int           `json:"failed"`
	}
	if err := json.Unmarshal(output.Bytes(), &summary); err != nil {
		t.Fatalf("decode batch summary: %v\n%s", err, output.String())
	}
	return summary
}

func TestRunBatchPreflightsJobsBeforeNetwork(t *testing.T) {
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests++ }))
	defer server.Close()
	t.Setenv("IMAGE_API_KEY", "test-key")
	outDir := t.TempDir()
	cases := []struct {
		name string
		jobs string
		want string
	}{
		{"missing operation", "{\"prompt\":\"one\",\"out\":\"one.png\"}\n", "operation"},
		{"unknown operation", "{\"operation\":\"other\",\"prompt\":\"one\",\"out\":\"one.png\"}\n", "unknown operation"},
		{"missing prompt", "{\"operation\":\"generate\",\"prompt\":\" \",\"out\":\"one.png\"}\n", "prompt"},
		{"missing output", "{\"operation\":\"generate\",\"prompt\":\"one\",\"out\":\" \"}\n", "out"},
		{"duplicate output", "{\"operation\":\"generate\",\"prompt\":\"one\",\"out\":\"same.png\"}\n{\"operation\":\"generate\",\"prompt\":\"two\",\"out\":\"./same.png\"}\n", "same output"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var output bytes.Buffer
			err := runBatch([]string{"--input", writeBatchInput(t, tc.jobs), "--out-dir", outDir, "--base-url", server.URL}, &output)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("runBatch error = %v, want %q", err, tc.want)
			}
		})
	}
	if requests != 0 {
		t.Fatalf("server received %d requests during preflight failures, want 0", requests)
	}
}

func TestRunBatchResolvesRelativeOutputsAndRejectsExistingBeforeNetwork(t *testing.T) {
	const png = "aGVsbG8="
	var requestsMu sync.Mutex
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestsMu.Lock()
		requests++
		requestsMu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"data":[{"b64_json":%q}]}`, png)
	}))
	defer server.Close()
	t.Setenv("IMAGE_API_KEY", "test-key")
	t.Setenv("IMAGE_API_MAX_ATTEMPTS", "1")
	root := t.TempDir()
	outDir := filepath.Join(root, "out")
	absolute := filepath.Join(root, "absolute.png")
	input := writeBatchInput(t, fmt.Sprintf("{\"operation\":\"generate\",\"prompt\":\"one\",\"out\":\"nested/one.png\"}\n{\"operation\":\"generate\",\"prompt\":\"two\",\"out\":%q}\n", absolute))
	var output bytes.Buffer
	if err := runBatch([]string{"--input", input, "--out-dir", outDir, "--base-url", server.URL, "--max-attempts", "1", "--concurrency", "2"}, &output); err != nil {
		t.Fatalf("runBatch returned an error: %v", err)
	}
	summary := decodeBatchSummary(t, &output)
	if got, want := summary.Jobs[0].Outputs[0], mustAbs(t, filepath.Join(outDir, "nested", "one.png")); got != want {
		t.Fatalf("relative output = %q, want %q", got, want)
	}
	if got, want := summary.Jobs[1].Outputs[0], mustAbs(t, absolute); got != want {
		t.Fatalf("absolute output = %q, want %q", got, want)
	}
	if err := os.WriteFile(filepath.Join(outDir, "existing.png"), []byte("existing"), 0644); err != nil {
		t.Fatal(err)
	}
	requestsMu.Lock()
	before := requests
	requestsMu.Unlock()
	err := runBatch([]string{"--input", writeBatchInput(t, "{\"operation\":\"generate\",\"prompt\":\"three\",\"out\":\"existing.png\"}\n"), "--out-dir", outDir, "--base-url", server.URL}, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "refusing to overwrite") {
		t.Fatalf("existing output error = %v", err)
	}
	requestsMu.Lock()
	after := requests
	requestsMu.Unlock()
	if after != before {
		t.Fatalf("existing output preflight sent a request")
	}
}

func mustAbs(t *testing.T, path string) string {
	t.Helper()
	absolute, err := filepath.Abs(path)
	if err != nil {
		t.Fatal(err)
	}
	return absolute
}

func TestRunBatchConcurrencyConfigPriority(t *testing.T) {
	t.Setenv("IMAGE_API_BATCH_CONCURRENCY", "2")
	input := writeBatchInput(t, "{\"operation\":\"generate\",\"prompt\":\"one\",\"out\":\"one.png\"}\n")
	for _, tc := range []struct {
		name string
		args []string
		want int
	}{
		{"environment", nil, 2},
		{"command line", []string{"--concurrency", "3"}, 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args := []string{"--input", input, "--out-dir", t.TempDir(), "--base-url", "http://example.invalid", "--dry-run"}
			args = append(args, tc.args...)
			var output bytes.Buffer
			if err := runBatch(args, &output); err != nil {
				t.Fatalf("runBatch returned an error: %v", err)
			}
			if got := decodeBatchSummary(t, &output).Concurrency; got != tc.want {
				t.Fatalf("concurrency = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestRunBatchUsesDynamicPoolAndKeepsInputOrder(t *testing.T) {
	const png = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII="
	var mu sync.Mutex
	starts := map[string]time.Time{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Prompt string `json:"prompt"`
		}
		_ = json.NewDecoder(r.Body).Decode(&payload)
		mu.Lock()
		starts[payload.Prompt] = time.Now()
		mu.Unlock()
		if payload.Prompt == "slow" {
			time.Sleep(120 * time.Millisecond)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"data":[{"b64_json":%q}]}`, png)
	}))
	defer server.Close()
	t.Setenv("IMAGE_API_KEY", "test-key")
	t.Setenv("IMAGE_API_MAX_ATTEMPTS", "1")
	input := writeBatchInput(t, "{\"operation\":\"generate\",\"prompt\":\"slow\",\"out\":\"slow.png\"}\n{\"operation\":\"generate\",\"prompt\":\"fast\",\"out\":\"fast.png\"}\n{\"operation\":\"generate\",\"prompt\":\"third\",\"out\":\"third.png\"}\n")
	var output bytes.Buffer
	if err := runBatch([]string{"--input", input, "--out-dir", t.TempDir(), "--base-url", server.URL, "--max-attempts", "1", "--concurrency", "2"}, &output); err != nil {
		t.Fatalf("runBatch returned an error: %v", err)
	}
	summary := decodeBatchSummary(t, &output)
	if len(summary.Jobs) != 3 || summary.Jobs[0].Index != 1 || summary.Jobs[1].Index != 2 || summary.Jobs[2].Index != 3 {
		t.Fatalf("summary jobs are not in input order: %+v", summary.Jobs)
	}
	mu.Lock()
	fast, third, slow := starts["fast"], starts["third"], starts["slow"]
	mu.Unlock()
	if third.IsZero() || fast.IsZero() || slow.IsZero() || !third.Before(slow.Add(100*time.Millisecond)) {
		t.Fatalf("third job did not start as soon as a worker freed: starts=%v", starts)
	}
}

func TestRunBatchContinuesAfterFailureAndFailFastStopsScheduling(t *testing.T) {
	const png = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII="
	var mu sync.Mutex
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Prompt string `json:"prompt"`
		}
		_ = json.NewDecoder(r.Body).Decode(&payload)
		mu.Lock()
		requests++
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if payload.Prompt == "bad" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		fmt.Fprintf(w, `{"data":[{"b64_json":%q}]}`, png)
	}))
	defer server.Close()
	t.Setenv("IMAGE_API_KEY", "test-key")
	t.Setenv("IMAGE_API_MAX_ATTEMPTS", "1")
	input := writeBatchInput(t, "{\"operation\":\"generate\",\"prompt\":\"bad\",\"out\":\"bad.png\"}\n{\"operation\":\"generate\",\"prompt\":\"good\",\"out\":\"good.png\"}\n")
	var output bytes.Buffer
	err := runBatch([]string{"--input", input, "--out-dir", t.TempDir(), "--base-url", server.URL, "--max-attempts", "1", "--concurrency", "1"}, &output)
	if err == nil {
		t.Fatal("runBatch succeeded despite a failed job")
	}
	summary := decodeBatchSummary(t, &output)
	if summary.Jobs[0].OK || !summary.Jobs[1].OK || len(summary.Jobs[1].Outputs) != 1 {
		t.Fatalf("default failure handling summary = %+v", summary.Jobs)
	}
	output.Reset()
	mu.Lock()
	requests = 0
	mu.Unlock()
	err = runBatch([]string{"--input", input, "--out-dir", t.TempDir(), "--base-url", server.URL, "--max-attempts", "1", "--concurrency", "1", "--fail-fast"}, &output)
	if err == nil {
		t.Fatal("fail-fast batch succeeded despite a failed job")
	}
	summary = decodeBatchSummary(t, &output)
	mu.Lock()
	gotRequests := requests
	mu.Unlock()
	if gotRequests != 1 || summary.Jobs[1].Error == "" {
		t.Fatalf("fail-fast scheduled too much or omitted skipped error: requests=%d jobs=%+v", gotRequests, summary.Jobs)
	}
}

func TestRunBatchMixedOperationsResolvesInputsAndPreservesMultipartOrder(t *testing.T) {
	const png = "aGVsbG8="
	root := t.TempDir()
	batchDir := filepath.Join(root, "jobs")
	if err := os.MkdirAll(batchDir, 0755); err != nil {
		t.Fatal(err)
	}
	for name, contents := range map[string]string{
		"first.png":  "first image",
		"second.png": "second image",
		"mask.png":   "mask image",
	} {
		if err := os.WriteFile(filepath.Join(batchDir, name), []byte(contents), 0644); err != nil {
			t.Fatal(err)
		}
	}

	type requestRecord struct {
		path       string
		prompt     string
		imageNames []string
		imageData  []string
		maskName   string
		maskData   string
	}
	var mu sync.Mutex
	var requests []requestRecord
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		record := requestRecord{path: r.URL.Path}
		if r.URL.Path == "/v1/images/generations" {
			var payload struct {
				Prompt string `json:"prompt"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Errorf("decode generation request: %v", err)
			} else {
				record.prompt = payload.Prompt
			}
		} else if r.URL.Path == "/v1/images/edits" {
			reader, err := r.MultipartReader()
			if err != nil {
				t.Errorf("create multipart reader: %v", err)
			} else {
				for {
					part, err := reader.NextPart()
					if errors.Is(err, io.EOF) {
						break
					}
					if err != nil {
						t.Errorf("read multipart part: %v", err)
						break
					}
					data, err := io.ReadAll(part)
					if err != nil {
						t.Errorf("read multipart data: %v", err)
						break
					}
					switch part.FormName() {
					case "prompt":
						record.prompt = string(data)
					case "image":
						record.imageNames = append(record.imageNames, part.FileName())
						record.imageData = append(record.imageData, string(data))
					case "mask":
						record.maskName = part.FileName()
						record.maskData = string(data)
					}
				}
			}
		} else {
			t.Errorf("unexpected request path %q", r.URL.Path)
		}
		mu.Lock()
		requests = append(requests, record)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"data":[{"b64_json":%q}]}`, png)
	}))
	defer server.Close()

	input := filepath.Join(batchDir, "jobs.jsonl")
	secondImage := filepath.Join(batchDir, "second.png")
	jobs := "{" + `"operation":"generate","prompt":"  keep this prompt  ","out":"generated.png"` + "}\n" +
		fmt.Sprintf("{\"operation\":\"edit\",\"prompt\":\"  keep edit prompt  \",\"image\":[\"first.png\",%q],\"mask\":\"mask.png\",\"out\":\"edited.png\"}\n", secondImage)
	if err := os.WriteFile(input, []byte(jobs), 0644); err != nil {
		t.Fatal(err)
	}
	outDir := filepath.Join(root, "output")
	t.Setenv("IMAGE_API_KEY", "test-key")
	t.Setenv("IMAGE_API_MAX_ATTEMPTS", "1")
	var output bytes.Buffer
	if err := runBatch([]string{
		"--input", input,
		"--out-dir", outDir,
		"--base-url", server.URL,
		"--max-attempts", "1",
		"--concurrency", "1",
	}, &output); err != nil {
		t.Fatalf("runBatch returned an error: %v", err)
	}
	summary := decodeBatchSummary(t, &output)
	if len(summary.Jobs) != 2 || summary.Jobs[0].Operation != "generate" || summary.Jobs[1].Operation != "edit" {
		t.Fatalf("summary operations = %+v, want generate/edit", summary.Jobs)
	}
	if !summary.Jobs[0].OK || !summary.Jobs[1].OK || len(summary.Jobs[0].Outputs) != 1 || len(summary.Jobs[1].Outputs) != 1 {
		t.Fatalf("summary does not contain successful outputs: %+v", summary.Jobs)
	}
	if _, err := os.Stat(filepath.Join(outDir, "generated.png")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(outDir, "edited.png")); err != nil {
		t.Fatal(err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(requests) != 2 {
		t.Fatalf("server received %d requests, want 2", len(requests))
	}
	if requests[0].path != "/v1/images/generations" || requests[0].prompt != "  keep this prompt  " {
		t.Fatalf("generation request = %+v, want unchanged prompt", requests[0])
	}
	editRequest := requests[1]
	if editRequest.path != "/v1/images/edits" || editRequest.prompt != "  keep edit prompt  " {
		t.Fatalf("edit request = %+v, want unchanged prompt", editRequest)
	}
	if got, want := editRequest.imageNames, []string{"first.png", "second.png"}; !equalStrings(got, want) {
		t.Fatalf("multipart image names = %v, want %v", got, want)
	}
	if got, want := editRequest.imageData, []string{"first image", "second image"}; !equalStrings(got, want) {
		t.Fatalf("multipart image data = %v, want %v", got, want)
	}
	if editRequest.maskName != "mask.png" || editRequest.maskData != "mask image" {
		t.Fatalf("multipart mask = %q/%q, want mask.png/mask image", editRequest.maskName, editRequest.maskData)
	}
}

func equalStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func TestRunBatchRejectsReferenceConstraintsBeforeNetwork(t *testing.T) {
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests++ }))
	defer server.Close()
	t.Setenv("IMAGE_API_KEY", "test-key")
	root := t.TempDir()
	batchDir := filepath.Join(root, "batch")
	if err := os.MkdirAll(batchDir, 0755); err != nil {
		t.Fatal(err)
	}
	valid := filepath.Join(batchDir, "valid.png")
	if err := os.WriteFile(valid, []byte("valid"), 0644); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		job  string
		want string
	}{
		{"missing operation", `{"prompt":"text","out":"one.png"}` + "\n", "must declare operation"},
		{"unknown operation", `{"operation":"other","prompt":"text","out":"one.png"}` + "\n", "unknown operation"},
		{"edit missing image", `{"operation":"edit","prompt":"text","out":"one.png"}` + "\n", "at least one image"},
		{"generate image forbidden", `{"operation":"generate","prompt":"text","image":["valid.png"],"out":"one.png"}` + "\n", "generate jobs must not include image"},
		{"generate empty image forbidden", `{"operation":"generate","prompt":"text","image":[],"out":"one.png"}` + "\n", "generate jobs must not include image"},
		{"generate mask forbidden", `{"operation":"generate","prompt":"text","mask":"mask.png","out":"one.png"}` + "\n", "generate jobs must not include mask"},
		{"generate empty mask forbidden", `{"operation":"generate","prompt":"text","mask":"","out":"one.png"}` + "\n", "generate jobs must not include mask"},
		{"edit missing image file", `{"operation":"edit","prompt":"text","image":["missing.png"],"out":"one.png"}` + "\n", "input file not found"},
		{"edit missing mask file", `{"operation":"edit","prompt":"text","image":["valid.png"],"mask":"missing-mask.png","out":"one.png"}` + "\n", "input file not found"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var output bytes.Buffer
			input := filepath.Join(batchDir, tc.name+".jsonl")
			if err := os.WriteFile(input, []byte(tc.job), 0644); err != nil {
				t.Fatal(err)
			}
			err := runBatch([]string{"--input", input, "--out-dir", filepath.Join(root, "out"), "--base-url", server.URL}, &output)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("runBatch error = %v, want %q", err, tc.want)
			}
		})
	}
	if requests != 0 {
		t.Fatalf("server received %d requests during preflight failures, want 0", requests)
	}
}

func TestRunBatchSummaryIncludesOperationForFailedJob(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/images/edits" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"data":[{"b64_json":"aGVsbG8="}]}`)
	}))
	defer server.Close()
	root := t.TempDir()
	inputDir := filepath.Join(root, "batch")
	if err := os.MkdirAll(inputDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(inputDir, "input.png"), []byte("input"), 0644); err != nil {
		t.Fatal(err)
	}
	input := filepath.Join(inputDir, "jobs.jsonl")
	jobs := "{" + `"operation":"generate","prompt":"good","out":"good.png"` + "}\n" +
		"{" + `"operation":"edit","prompt":"bad","image":["input.png"],"out":"bad.png"` + "}\n"
	if err := os.WriteFile(input, []byte(jobs), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("IMAGE_API_KEY", "test-key")
	t.Setenv("IMAGE_API_MAX_ATTEMPTS", "1")
	var output bytes.Buffer
	err := runBatch([]string{"--input", input, "--out-dir", filepath.Join(root, "out"), "--base-url", server.URL, "--max-attempts", "1", "--concurrency", "1"}, &output)
	if err == nil {
		t.Fatal("runBatch succeeded despite failed edit")
	}
	summary := decodeBatchSummary(t, &output)
	if summary.Jobs[0].Operation != "generate" || summary.Jobs[1].Operation != "edit" {
		t.Fatalf("summary operations = %+v, want generate/edit", summary.Jobs)
	}
	if !summary.Jobs[0].OK || summary.Jobs[1].OK || summary.Jobs[1].Error == "" {
		t.Fatalf("summary success/failure = %+v, want output and failure reason", summary.Jobs)
	}
}
