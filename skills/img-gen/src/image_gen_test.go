package main

import (
	"bytes"
	"encoding/base64"
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

func TestMain(m *testing.M) {
	home, err := os.MkdirTemp("", "img-gen-tests-home-")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(home)
	if err := os.Setenv("HOME", home); err != nil {
		panic(err)
	}
	path, err := configFilePath()
	if err != nil {
		panic(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		panic(err)
	}
	config := `{"defaultProvider":"openai","providers":{"openai":{"baseURL":"http://config.invalid","apiKey":"test-key","model":"gpt-image-2"}}}`
	if err := os.WriteFile(path, []byte(config), 0600); err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}

func writeUserConfig(t *testing.T, contents string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	path, err := configFilePath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
		t.Fatal(err)
	}
}

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

func TestInitCreatesUserTemplateForBothProvidersWithoutEchoingSecrets(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	var output bytes.Buffer
	if err := runInit(nil, &output); err != nil {
		t.Fatalf("runInit returned an error: %v", err)
	}
	path, err := configFilePath()
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("configuration template was not created: %v", err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("configuration permissions = %o, want 600", info.Mode().Perm())
	}
	var config userConfig
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("decode configuration template: %v", err)
	}
	if config.DefaultProvider != "openai" {
		t.Fatalf("defaultProvider = %q, want openai", config.DefaultProvider)
	}
	for _, provider := range []string{"openai", "google"} {
		selected, ok := config.Providers[provider]
		if !ok {
			t.Fatalf("configuration template is missing %q", provider)
		}
		if selected.BaseURL == "" || selected.Model == "" || selected.APIKey != "" {
			t.Fatalf("%s template configuration = %+v, want baseURL/model and empty apiKey", provider, selected)
		}
	}
	if strings.Contains(output.String(), "apiKey") || strings.Contains(output.String(), "secret") {
		t.Fatalf("initialization output exposed configuration details: %s", output.String())
	}
	if !strings.Contains(output.String(), path) || !strings.Contains(output.String(), "fill") {
		t.Fatalf("initialization output = %s, want path and local fill guidance", output.String())
	}
}

func TestInitRefusesToOverwriteExistingConfigurationByDefault(t *testing.T) {
	contents := `{"defaultProvider":"google","providers":{"google":{"baseURL":"https://example.invalid","apiKey":"local-only","model":"example-model"}}}`
	writeUserConfig(t, contents)
	var output bytes.Buffer
	err := runInit(nil, &output)
	if err == nil || !strings.Contains(err.Error(), "refusing to overwrite") {
		t.Fatalf("runInit error = %v, want overwrite refusal", err)
	}
	path, err := configFilePath()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != contents {
		t.Fatalf("existing Provider Configuration changed: %s", data)
	}
	if output.Len() != 0 {
		t.Fatalf("failed initialization wrote output: %s", output.String())
	}
}

func TestMissingConfigurationProvidesInitializationGuidanceWithoutNetwork(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
	}))
	defer server.Close()
	var output bytes.Buffer
	err := runGenerate([]string{"--provider", "openai", "--base-url", server.URL, "--prompt", "missing configuration"}, &output)
	if err == nil || !strings.Contains(err.Error(), "img-gen init") {
		t.Fatalf("runGenerate error = %v, want initialization command guidance", err)
	}
	if requests != 0 {
		t.Fatalf("server received %d requests with missing configuration, want 0", requests)
	}
	if strings.Contains(output.String(), "apiKey") || strings.Contains(output.String(), "API Key") {
		t.Fatalf("missing configuration output exposed a key field: %s", output.String())
	}
	if !strings.Contains(output.String(), "img-gen init") || !strings.Contains(output.String(), "fill") {
		t.Fatalf("missing configuration output = %s, want safe initialization guidance", output.String())
	}
}

func TestProviderConfigurationSelectsDefaultAndUsesOpenAIProtocol(t *testing.T) {
	const png = "aGVsbG8="
	var authorization, model string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorization = r.Header.Get("Authorization")
		var payload struct {
			Model string `json:"model"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		model = payload.Model
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"data":[{"b64_json":%q}]}`, png)
	}))
	defer server.Close()
	writeUserConfig(t, fmt.Sprintf(`{"defaultProvider":"openai","providers":{"openai":{"baseURL":%q,"apiKey":"json-key","model":"json-model"}}}`, server.URL))
	var output bytes.Buffer
	if err := runGenerate([]string{"--prompt", "configured image", "--out-dir", t.TempDir(), "--max-attempts", "1"}, &output); err != nil {
		t.Fatalf("runGenerate returned an error: %v", err)
	}
	if authorization != "Bearer json-key" {
		t.Fatalf("Authorization = %q, want JSON Provider Configuration key", authorization)
	}
	if model != "json-model" {
		t.Fatalf("model = %q, want JSON Provider Configuration model", model)
	}
	if !strings.Contains(output.String(), `"model": "json-model"`) || !strings.Contains(output.String(), `"provider": "openai"`) {
		t.Fatalf("success output = %s, want Provider and Model", output.String())
	}
}

func TestExplicitProviderOverridesConfiguredDefault(t *testing.T) {
	const png = "aGVsbG8="
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"data":[{"b64_json":%q}]}`, png)
	}))
	defer server.Close()
	writeUserConfig(t, fmt.Sprintf(`{"defaultProvider":"google","providers":{"openai":{"baseURL":%q,"apiKey":"openai-key","model":"openai-model"},"google":{"baseURL":"http://unused.invalid","apiKey":"google-key","model":"google-model"}}}`, server.URL))
	var output bytes.Buffer
	if err := runGenerate([]string{"--provider", "openai", "--prompt", "explicit provider", "--out-dir", t.TempDir(), "--max-attempts", "1"}, &output); err != nil {
		t.Fatalf("explicit openai Provider Selection failed: %v", err)
	}
	if !strings.Contains(output.String(), `"provider": "openai"`) {
		t.Fatalf("success output = %s, want openai Provider", output.String())
	}
}

func TestProviderSelectionFailsWithoutDefaultOrExplicitProvider(t *testing.T) {
	writeUserConfig(t, `{"providers":{"openai":{"baseURL":"http://unused.invalid","apiKey":"json-key","model":"json-model"}}}`)
	t.Setenv("IMAGE_API_BASE_URL", "http://legacy.invalid")
	t.Setenv("IMAGE_API_KEY", "legacy-key")
	t.Setenv("IMAGE_API_MODEL", "legacy-model")
	err := runGenerate([]string{"--prompt", "missing provider", "--out-dir", t.TempDir()}, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "no Provider selected") {
		t.Fatalf("missing Provider error = %v, want clear selection failure", err)
	}
}

func TestProviderConfigurationDoesNotFallbackToLegacyEnvironment(t *testing.T) {
	writeUserConfig(t, ``)
	t.Setenv("IMAGE_API_BASE_URL", "http://legacy.invalid")
	t.Setenv("IMAGE_API_KEY", "legacy-key")
	t.Setenv("IMAGE_API_MODEL", "legacy-model")
	err := runGenerate([]string{"--provider", "openai", "--prompt", "legacy values", "--base-url", "http://override.invalid"}, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "invalid JSON") {
		t.Fatalf("legacy fallback error = %v, want configuration error", err)
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

func TestRunBatchUsesExplicitJobProviderAndReportsProviderModel(t *testing.T) {
	const png = "aGVsbG8="
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"data":[{"b64_json":%q}]}`, png)
	}))
	defer server.Close()
	writeUserConfig(t, fmt.Sprintf(`{"defaultProvider":"google","providers":{"openai":{"baseURL":%q,"apiKey":"openai-key","model":"openai-model"},"google":{"baseURL":"http://unused.invalid","apiKey":"google-key","model":"google-model"}}}`, server.URL))
	input := writeBatchInput(t, `{"operation":"generate","provider":"openai","prompt":"batch image","out":"batch.png"}`+"\n")
	var output bytes.Buffer
	if err := runBatch([]string{"--input", input, "--out-dir", t.TempDir(), "--concurrency", "1", "--max-attempts", "1"}, &output); err != nil {
		t.Fatalf("runBatch returned an error: %v", err)
	}
	summary := decodeBatchSummary(t, &output)
	if summary.Jobs[0].Provider != "openai" || summary.Jobs[0].Model != "openai-model" {
		t.Fatalf("batch Provider/Model = %q/%q, want openai/openai-model", summary.Jobs[0].Provider, summary.Jobs[0].Model)
	}
}

func TestRunBatchCommandProviderAppliesToJobsWithoutJobProvider(t *testing.T) {
	const png = "aGVsbG8="
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"data":[{"b64_json":%q}]}`, png)
	}))
	defer server.Close()
	writeUserConfig(t, fmt.Sprintf(`{"defaultProvider":"google","providers":{"openai":{"baseURL":%q,"apiKey":"openai-key","model":"openai-model"},"google":{"baseURL":"http://unused.invalid","apiKey":"google-key","model":"google-model"}}}`, server.URL))
	input := writeBatchInput(t, "{\"operation\":\"generate\",\"prompt\":\"command provider\",\"out\":\"one.png\"}\n")
	var output bytes.Buffer
	if err := runBatch([]string{"--input", input, "--out-dir", t.TempDir(), "--concurrency", "1", "--max-attempts", "1", "--provider", "openai"}, &output); err != nil {
		t.Fatalf("runBatch with command-level Provider Selection failed: %v", err)
	}
	summary := decodeBatchSummary(t, &output)
	if summary.Jobs[0].Provider != "openai" || summary.Jobs[0].Model != "openai-model" {
		t.Fatalf("batch Provider/Model = %q/%q, want command-level openai/openai-model", summary.Jobs[0].Provider, summary.Jobs[0].Model)
	}
}

func TestRunBatchJobModelOverrideReachesRequest(t *testing.T) {
	const png = "aGVsbG8="
	var model string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Model string `json:"model"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode request: %v", err)
		}
		model = payload.Model
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"data":[{"b64_json":%q}]}`, png)
	}))
	defer server.Close()
	writeUserConfig(t, fmt.Sprintf(`{"defaultProvider":"openai","providers":{"openai":{"baseURL":%q,"apiKey":"json-key","model":"json-model"}}}`, server.URL))
	input := writeBatchInput(t, "{\"operation\":\"generate\",\"prompt\":\"job model\",\"model\":\"job-model\",\"out\":\"one.png\"}\n")
	var output bytes.Buffer
	if err := runBatch([]string{"--input", input, "--out-dir", t.TempDir(), "--concurrency", "1", "--max-attempts", "1"}, &output); err != nil {
		t.Fatalf("runBatch returned an error: %v", err)
	}
	if model != "job-model" {
		t.Fatalf("request model = %q, want job-level override job-model", model)
	}
	summary := decodeBatchSummary(t, &output)
	if summary.Jobs[0].Model != "job-model" {
		t.Fatalf("summary model = %q, want job-model", summary.Jobs[0].Model)
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

func TestRunBatchConcurrencyIgnoresLegacyEnvironment(t *testing.T) {
	t.Setenv("IMAGE_API_BATCH_CONCURRENCY", "2")
	input := writeBatchInput(t, "{\"operation\":\"generate\",\"prompt\":\"one\",\"out\":\"one.png\"}\n")
	for _, tc := range []struct {
		name string
		args []string
		want int
	}{
		{"default", nil, 5},
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

func writeProviderConfig(t *testing.T, serverURL string) {
	t.Helper()
	writeUserConfig(t, fmt.Sprintf(`{"defaultProvider":"openai","providers":{"openai":{"baseURL":%q,"apiKey":"fake-test-key","model":"fake-model"}}}`, serverURL))
}

func decodeFailureResult(t *testing.T, output *bytes.Buffer) failureResult {
	t.Helper()
	var failure failureResult
	if err := json.Unmarshal(output.Bytes(), &failure); err != nil {
		t.Fatalf("decode failure result: %v\n%s", err, output.String())
	}
	return failure
}

func TestGoogleProviderUsesGoogleProtocolAndConfiguration(t *testing.T) {
	const png = "aGVsbG8="
	var authorization string
	var payload struct {
		Instances []struct {
			Prompt string `json:"prompt"`
		} `json:"instances"`
		Parameters struct {
			SampleCount int `json:"sampleCount"`
		} `json:"parameters"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1beta/models/google-imagen:predict" {
			t.Errorf("Google request path = %q, want /v1beta/models/google-imagen:predict", r.URL.Path)
		}
		if got := r.URL.Query().Get("key"); got != "google-key" {
			t.Errorf("Google API key query = %q, want google-key", got)
		}
		authorization = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode Google request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"predictions":[{"bytesBase64Encoded":%q}]}`, png)
	}))
	defer server.Close()
	writeUserConfig(t, fmt.Sprintf(`{"defaultProvider":"google","providers":{"openai":{"baseURL":"http://openai.invalid","apiKey":"openai-key","model":"openai-model"},"google":{"baseURL":%q,"apiKey":"google-key","model":"google-imagen"}}}`, server.URL))
	var output bytes.Buffer
	outDir := t.TempDir()
	if err := runGenerate([]string{"--prompt", "a Google image", "--out-dir", outDir, "--max-attempts", "1"}, &output); err != nil {
		t.Fatalf("runGenerate returned an error: %v", err)
	}
	if authorization != "" {
		t.Fatalf("Google request sent Authorization header %q", authorization)
	}
	if len(payload.Instances) != 1 || payload.Instances[0].Prompt != "a Google image" {
		t.Fatalf("Google instances = %+v, want one unchanged prompt", payload.Instances)
	}
	if payload.Parameters.SampleCount != 1 {
		t.Fatalf("Google sampleCount = %d, want 1", payload.Parameters.SampleCount)
	}
	var result struct {
		Provider string   `json:"provider"`
		Model    string   `json:"model"`
		Outputs  []string `json:"outputs"`
	}
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatalf("decode Google output: %v", err)
	}
	if result.Provider != "google" || result.Model != "google-imagen" || len(result.Outputs) != 1 {
		t.Fatalf("Google result = %+v, want provider/model and one output", result)
	}
	if data, err := os.ReadFile(result.Outputs[0]); err != nil || string(data) != "hello" {
		t.Fatalf("Google output data = %q, read error = %v, want hello", data, err)
	}
}

func TestGoogleProviderEditUsesJSONImageProtocol(t *testing.T) {
	const png = "aGVsbG8="
	root := t.TempDir()
	input := filepath.Join(root, "input.png")
	mask := filepath.Join(root, "mask.png")
	if err := os.WriteFile(input, []byte("input image"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mask, []byte("mask image"), 0644); err != nil {
		t.Fatal(err)
	}
	var payload struct {
		Instances []struct {
			Prompt string `json:"prompt"`
			Image  struct {
				Bytes string `json:"bytesBase64Encoded"`
			} `json:"image"`
			Mask struct {
				Image struct {
					Bytes string `json:"bytesBase64Encoded"`
				} `json:"image"`
			} `json:"mask"`
		} `json:"instances"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1beta/models/google-editor:predict" {
			t.Errorf("Google edit path = %q, want /v1beta/models/google-editor:predict", r.URL.Path)
		}
		if got := r.URL.Query().Get("key"); got != "google-edit-key" {
			t.Errorf("Google edit API key query = %q, want google-edit-key", got)
		}
		if got := r.Header.Get("Authorization"); got != "" {
			t.Errorf("Google edit sent Authorization header %q", got)
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode Google edit request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"predictions":[{"bytesBase64Encoded":%q}]}`, png)
	}))
	defer server.Close()
	writeUserConfig(t, fmt.Sprintf(`{"defaultProvider":"google","providers":{"google":{"baseURL":%q,"apiKey":"google-edit-key","model":"google-editor"}}}`, server.URL))
	var output bytes.Buffer
	if err := runEdit([]string{"--image", input, "--mask", mask, "--prompt", "edit with Google", "--out-dir", filepath.Join(root, "out"), "--max-attempts", "1"}, &output); err != nil {
		t.Fatalf("runEdit returned an error: %v", err)
	}
	if len(payload.Instances) != 1 || payload.Instances[0].Prompt != "edit with Google" {
		t.Fatalf("Google edit instances = %+v, want one unchanged prompt", payload.Instances)
	}
	if payload.Instances[0].Image.Bytes != base64.StdEncoding.EncodeToString([]byte("input image")) {
		t.Fatalf("Google edit image bytes = %q", payload.Instances[0].Image.Bytes)
	}
	if payload.Instances[0].Mask.Image.Bytes != base64.StdEncoding.EncodeToString([]byte("mask image")) {
		t.Fatalf("Google edit mask bytes = %q", payload.Instances[0].Mask.Image.Bytes)
	}
}

func TestGoogleProviderFailuresUseCommonRetryRules(t *testing.T) {
	const png = "aGVsbG8="
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if requests == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"predictions":[{"bytesBase64Encoded":%q}]}`, png)
	}))
	defer server.Close()
	writeUserConfig(t, fmt.Sprintf(`{"defaultProvider":"google","providers":{"google":{"baseURL":%q,"apiKey":"google-key","model":"google-model"}}}`, server.URL))
	var output bytes.Buffer
	if err := runGenerate([]string{"--prompt", "retry Google", "--out-dir", t.TempDir(), "--max-attempts", "2"}, &output); err != nil {
		t.Fatalf("Google 5xx retry returned an error: %v", err)
	}
	if requests != 2 {
		t.Fatalf("Google 5xx request count = %d, want 2", requests)
	}

	requests = 0
	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"error":"google-key must not be reported"}`)
	}))
	defer server2.Close()
	writeUserConfig(t, fmt.Sprintf(`{"defaultProvider":"google","providers":{"google":{"baseURL":%q,"apiKey":"google-key","model":"google-model"}}}`, server2.URL))
	output.Reset()
	err := runGenerate([]string{"--prompt", "no Google 4xx retry", "--out-dir", t.TempDir(), "--max-attempts", "3"}, &output)
	if err == nil || !strings.Contains(err.Error(), "HTTP 401") {
		t.Fatalf("Google 4xx error = %v, want HTTP 401", err)
	}
	if requests != 1 {
		t.Fatalf("Google 4xx request count = %d, want 1", requests)
	}
	if strings.Contains(output.String(), "google-key") {
		t.Fatalf("Google failure leaked API key: %s", output.String())
	}
}

func TestGoogleProviderBatchUsesCommonOutputAndSummary(t *testing.T) {
	const png = "aGVsbG8="
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Query().Get("key") != "batch-google-key" {
			t.Errorf("batch Google API key = %q, want batch-google-key", r.URL.Query().Get("key"))
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"predictions":[{"bytesBase64Encoded":%q}]}`, png)
	}))
	defer server.Close()
	writeUserConfig(t, fmt.Sprintf(`{"defaultProvider":"google","providers":{"google":{"baseURL":%q,"apiKey":"batch-google-key","model":"batch-google-model"}}}`, server.URL))
	input := writeBatchInput(t, `{"operation":"generate","prompt":"batch Google image","out":"google.png"}`+"\n")
	outDir := t.TempDir()
	var output bytes.Buffer
	if err := runBatch([]string{"--input", input, "--out-dir", outDir, "--concurrency", "1", "--max-attempts", "1"}, &output); err != nil {
		t.Fatalf("runBatch returned an error: %v", err)
	}
	summary := decodeBatchSummary(t, &output)
	if requests != 1 || len(summary.Jobs) != 1 || !summary.Jobs[0].OK || summary.Jobs[0].Provider != "google" || summary.Jobs[0].Model != "batch-google-model" {
		t.Fatalf("Google batch requests/summary = %d/%+v", requests, summary.Jobs)
	}
	if len(summary.Jobs[0].Outputs) != 1 {
		t.Fatalf("Google batch outputs = %+v, want one output", summary.Jobs[0].Outputs)
	}
	if _, err := os.Stat(summary.Jobs[0].Outputs[0]); err != nil {
		t.Fatalf("Google batch output was not saved: %v", err)
	}
}

func TestExplicitGoogleProviderDoesNotUseOpenAIConfiguration(t *testing.T) {
	const png = "aGVsbG8="
	var googleRequests, openAIRequests int
	googleServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		googleRequests++
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"predictions":[{"bytesBase64Encoded":%q}]}`, png)
	}))
	defer googleServer.Close()
	openAIServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		openAIRequests++
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"data":[{"b64_json":%q}]}`, png)
	}))
	defer openAIServer.Close()
	writeUserConfig(t, fmt.Sprintf(`{"defaultProvider":"openai","providers":{"openai":{"baseURL":%q,"apiKey":"openai-key","model":"openai-model"},"google":{"baseURL":%q,"apiKey":"google-key","model":"google-model"}}}`, openAIServer.URL, googleServer.URL))
	var output bytes.Buffer
	if err := runGenerate([]string{"--provider", "google", "--prompt", "explicit Google", "--out-dir", t.TempDir(), "--max-attempts", "1"}, &output); err != nil {
		t.Fatalf("explicit Google Provider Selection failed: %v", err)
	}
	if googleRequests != 1 || openAIRequests != 0 {
		t.Fatalf("Provider isolation requests = Google %d, OpenAI %d; want 1, 0", googleRequests, openAIRequests)
	}
}

func TestGenerateRetries5xxWithinBoundedAttempts(t *testing.T) {
	var mu sync.Mutex
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests++
		mu.Unlock()
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	writeProviderConfig(t, server.URL)
	var output bytes.Buffer
	err := runGenerate([]string{"--prompt", "retry me", "--out-dir", t.TempDir(), "--max-attempts", "3"}, &output)
	if err == nil || !strings.Contains(err.Error(), "API request failed with HTTP 500") {
		t.Fatalf("runGenerate error = %v, want bounded HTTP 500 failure", err)
	}
	mu.Lock()
	got := requests
	mu.Unlock()
	if got != 3 {
		t.Fatalf("server received %d requests, want 3 bounded attempts", got)
	}
	failure := decodeFailureResult(t, &output)
	if failure.Provider != "openai" || failure.Model != "fake-model" {
		t.Fatalf("failure Provider/Model = %q/%q, want openai/fake-model", failure.Provider, failure.Model)
	}
	if failure.Status != 500 || failure.Error != "API request failed with HTTP 500" {
		t.Fatalf("failure result = %+v, want HTTP 500 status and safe message", failure)
	}
}

func TestGenerateRecoversAfter5xxRetry(t *testing.T) {
	const png = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII="
	var mu sync.Mutex
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests++
		count := requests
		mu.Unlock()
		if count == 1 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"data":[{"b64_json":%q}]}`, png)
	}))
	defer server.Close()
	writeProviderConfig(t, server.URL)
	outDir := t.TempDir()
	var output bytes.Buffer
	if err := runGenerate([]string{"--prompt", "flaky provider", "--out-dir", outDir, "--max-attempts", "3"}, &output); err != nil {
		t.Fatalf("runGenerate did not recover after a 5xx retry: %v", err)
	}
	mu.Lock()
	got := requests
	mu.Unlock()
	if got != 2 {
		t.Fatalf("server received %d requests, want one 5xx attempt plus one retry", got)
	}
	var result struct {
		Provider string   `json:"provider"`
		Model    string   `json:"model"`
		Outputs  []string `json:"outputs"`
	}
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatalf("decode success result: %v\n%s", err, output.String())
	}
	if result.Provider != "openai" || result.Model != "fake-model" || len(result.Outputs) != 1 {
		t.Fatalf("success result = %+v, want openai/fake-model with one output", result)
	}
	if _, err := os.Stat(result.Outputs[0]); err != nil {
		t.Fatalf("recovered output %q is not saved: %v", result.Outputs[0], err)
	}
}

func TestSingleImageCommandsDoNotRetryAny4xx(t *testing.T) {
	for _, status := range []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusTooManyRequests} {
		for _, command := range []string{"generate", "edit"} {
			t.Run(fmt.Sprintf("%s-%d", command, status), func(t *testing.T) {
				var mu sync.Mutex
				var requests int
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					mu.Lock()
					requests++
					mu.Unlock()
					w.Header().Set("Retry-After", "0")
					w.WriteHeader(status)
				}))
				defer server.Close()
				writeProviderConfig(t, server.URL)
				var argv []string
				if command == "edit" {
					input := filepath.Join(t.TempDir(), "input.png")
					if err := os.WriteFile(input, []byte("input image"), 0644); err != nil {
						t.Fatal(err)
					}
					argv = append(argv, "--image", input)
				}
				argv = append(argv, "--prompt", "rejected request", "--out-dir", t.TempDir(), "--max-attempts", "3")
				var output bytes.Buffer
				var err error
				if command == "generate" {
					err = runGenerate(argv, &output)
				} else {
					err = runEdit(argv, &output)
				}
				want := fmt.Sprintf("API request failed with HTTP %d", status)
				if err == nil || !strings.Contains(err.Error(), want) {
					t.Fatalf("%s error = %v, want immediate %q failure", command, err, want)
				}
				mu.Lock()
				got := requests
				mu.Unlock()
				if got != 1 {
					t.Fatalf("%s made %d requests for HTTP %d, want exactly 1", command, got, status)
				}
				failure := decodeFailureResult(t, &output)
				if failure.Provider != "openai" || failure.Model != "fake-model" {
					t.Fatalf("failure Provider/Model = %q/%q, want openai/fake-model", failure.Provider, failure.Model)
				}
				if failure.Status != status || failure.Error != want {
					t.Fatalf("failure result = %+v, want status %d with %q", failure, status, want)
				}
			})
		}
	}
}

func TestGenerateTimeoutRetriesWithinBoundedAttempts(t *testing.T) {
	var mu sync.Mutex
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests++
		mu.Unlock()
		time.Sleep(150 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"data":[{"b64_json":"aGVsbG8="}]}`)
	}))
	defer server.Close()
	writeProviderConfig(t, server.URL)
	var output bytes.Buffer
	err := runGenerate([]string{"--prompt", "too slow", "--out-dir", t.TempDir(), "--timeout", "0.05", "--max-attempts", "2"}, &output)
	if err == nil || !strings.Contains(err.Error(), "network error or timeout") {
		t.Fatalf("runGenerate error = %v, want timeout failure", err)
	}
	mu.Lock()
	got := requests
	mu.Unlock()
	if got != 2 {
		t.Fatalf("server received %d requests, want 2 bounded timeout attempts", got)
	}
	failure := decodeFailureResult(t, &output)
	if failure.Provider != "openai" || failure.Model != "fake-model" {
		t.Fatalf("failure Provider/Model = %q/%q, want openai/fake-model", failure.Provider, failure.Model)
	}
	if failure.Status != 0 {
		t.Fatalf("failure status = %d, want no HTTP status for a timeout", failure.Status)
	}
	if failure.Error != "API request failed: network error or timeout" {
		t.Fatalf("failure error = %q, want safe timeout message", failure.Error)
	}
}

func TestGenerateNetworkFailureFailsWithoutHTTPStatus(t *testing.T) {
	writeProviderConfig(t, "http://127.0.0.1:1")
	var output bytes.Buffer
	err := runGenerate([]string{"--prompt", "unreachable", "--out-dir", t.TempDir(), "--max-attempts", "1"}, &output)
	if err == nil || !strings.Contains(err.Error(), "network error or timeout") {
		t.Fatalf("runGenerate error = %v, want network failure", err)
	}
	failure := decodeFailureResult(t, &output)
	if failure.Provider != "openai" || failure.Model != "fake-model" {
		t.Fatalf("failure Provider/Model = %q/%q, want openai/fake-model", failure.Provider, failure.Model)
	}
	if failure.Status != 0 || failure.Error != "API request failed: network error or timeout" {
		t.Fatalf("failure result = %+v, want no HTTP status and safe network message", failure)
	}
}

func TestGenerateFailureResultDoesNotExposeProviderErrorDetails(t *testing.T) {
	const fakeKey = "fake-leaky-key-12345"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprintf(w, `{"error":{"message":"invalid API key %q","type":"invalid_request_error"},"echoed_authorization":%q}`, fakeKey, "Bearer "+fakeKey)
	}))
	defer server.Close()
	writeUserConfig(t, fmt.Sprintf(`{"defaultProvider":"openai","providers":{"openai":{"baseURL":%q,"apiKey":%q,"model":"fake-model"}}}`, server.URL, fakeKey))
	var output bytes.Buffer
	err := runGenerate([]string{"--prompt", "secret check", "--out-dir", t.TempDir(), "--max-attempts", "1"}, &output)
	if err == nil || err.Error() != "API request failed with HTTP 401" {
		t.Fatalf("runGenerate error = %v, want only the HTTP status message", err)
	}
	failure := decodeFailureResult(t, &output)
	if failure.Provider != "openai" || failure.Model != "fake-model" {
		t.Fatalf("failure Provider/Model = %q/%q, want openai/fake-model", failure.Provider, failure.Model)
	}
	if failure.Status != 401 || failure.Error != "API request failed with HTTP 401" {
		t.Fatalf("failure result = %+v, want status 401 with the safe message", failure)
	}
	for _, secret := range []string{fakeKey, "Bearer", "invalid_request_error", "Authorization"} {
		if strings.Contains(output.String(), secret) {
			t.Fatalf("failure output leaked %q: %s", secret, output.String())
		}
	}
}

func TestRunBatch4xxJobsFailOnceAndPreservePartialSuccess(t *testing.T) {
	const png = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII="
	var mu sync.Mutex
	requests := map[string]int{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Prompt string `json:"prompt"`
		}
		_ = json.NewDecoder(r.Body).Decode(&payload)
		mu.Lock()
		requests[payload.Prompt]++
		mu.Unlock()
		switch payload.Prompt {
		case "rejected-429":
			w.WriteHeader(http.StatusTooManyRequests)
		case "rejected-403":
			w.WriteHeader(http.StatusForbidden)
		default:
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"data":[{"b64_json":%q}]}`, png)
		}
	}))
	defer server.Close()
	writeProviderConfig(t, server.URL)
	input := writeBatchInput(t, "{\"operation\":\"generate\",\"prompt\":\"rejected-429\",\"out\":\"one.png\"}\n{\"operation\":\"generate\",\"prompt\":\"succeeds\",\"out\":\"two.png\"}\n{\"operation\":\"generate\",\"prompt\":\"rejected-403\",\"out\":\"three.png\"}\n")
	var output bytes.Buffer
	err := runBatch([]string{"--input", input, "--out-dir", t.TempDir(), "--concurrency", "1", "--max-attempts", "3"}, &output)
	if err == nil || !strings.Contains(err.Error(), "one or more batch jobs failed") {
		t.Fatalf("runBatch error = %v, want nonzero batch failure", err)
	}
	mu.Lock()
	counts := make(map[string]int, len(requests))
	for prompt, count := range requests {
		counts[prompt] = count
	}
	mu.Unlock()
	if counts["rejected-429"] != 1 || counts["rejected-403"] != 1 || counts["succeeds"] != 1 {
		t.Fatalf("request counts = %v, want exactly one attempt per job despite --max-attempts 3", counts)
	}
	summary := decodeBatchSummary(t, &output)
	if summary.Succeeded != 1 || summary.Failed != 2 {
		t.Fatalf("summary succeeded/failed = %d/%d, want 1/2", summary.Succeeded, summary.Failed)
	}
	first, second, third := summary.Jobs[0], summary.Jobs[1], summary.Jobs[2]
	if first.Index != 1 || second.Index != 2 || third.Index != 3 {
		t.Fatalf("summary jobs are not in input order: %+v", summary.Jobs)
	}
	if first.OK || first.Status != http.StatusTooManyRequests || !strings.Contains(first.Error, "HTTP 429") {
		t.Fatalf("429 job result = %+v, want failure with status 429", first)
	}
	if !second.OK || len(second.Outputs) != 1 {
		t.Fatalf("successful job result = %+v, want outputs", second)
	}
	if third.OK || third.Status != http.StatusForbidden || !strings.Contains(third.Error, "HTTP 403") {
		t.Fatalf("403 job result = %+v, want failure with status 403", third)
	}
	for _, job := range summary.Jobs {
		if job.Provider != "openai" || job.Model != "fake-model" {
			t.Fatalf("job %d Provider/Model = %q/%q, want openai/fake-model", job.Index, job.Provider, job.Model)
		}
	}
}

func TestRunBatch5xxRetriesWithinBoundedAttempts(t *testing.T) {
	const png = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII="
	var mu sync.Mutex
	requests := map[string]int{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Prompt string `json:"prompt"`
		}
		_ = json.NewDecoder(r.Body).Decode(&payload)
		mu.Lock()
		requests[payload.Prompt]++
		mu.Unlock()
		if payload.Prompt == "server-error" {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"data":[{"b64_json":%q}]}`, png)
	}))
	defer server.Close()
	writeProviderConfig(t, server.URL)
	input := writeBatchInput(t, "{\"operation\":\"generate\",\"prompt\":\"server-error\",\"out\":\"one.png\"}\n{\"operation\":\"generate\",\"prompt\":\"succeeds\",\"out\":\"two.png\"}\n")
	var output bytes.Buffer
	err := runBatch([]string{"--input", input, "--out-dir", t.TempDir(), "--concurrency", "1", "--max-attempts", "3"}, &output)
	if err == nil || !strings.Contains(err.Error(), "one or more batch jobs failed") {
		t.Fatalf("runBatch error = %v, want nonzero batch failure", err)
	}
	mu.Lock()
	flaky, good := requests["server-error"], requests["succeeds"]
	mu.Unlock()
	if flaky != 3 || good != 1 {
		t.Fatalf("request counts = server-error:%d succeeds:%d, want 3 bounded 5xx attempts and 1 success", flaky, good)
	}
	summary := decodeBatchSummary(t, &output)
	if summary.Succeeded != 1 || summary.Failed != 1 {
		t.Fatalf("summary succeeded/failed = %d/%d, want partial success 1/1", summary.Succeeded, summary.Failed)
	}
	if summary.Jobs[0].OK || summary.Jobs[0].Status != http.StatusInternalServerError || !strings.Contains(summary.Jobs[0].Error, "HTTP 500") {
		t.Fatalf("5xx job result = %+v, want failure with status 500 after retries", summary.Jobs[0])
	}
	if !summary.Jobs[1].OK || len(summary.Jobs[1].Outputs) != 1 {
		t.Fatalf("successful job result = %+v, want outputs", summary.Jobs[1])
	}
}
