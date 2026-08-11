package main

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math/rand"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	defaultModel            = "gpt-image-2"
	defaultSize             = "auto"
	defaultQuality          = "auto"
	defaultOutDir           = "output/imagegen"
	defaultMaxAttempts      = 3
	defaultBatchConcurrency = 5
	retryBaseDelay          = 750 * time.Millisecond
	retryMaxDelay           = 30 * time.Second
)

var retryable = map[int]bool{429: true, 500: true, 502: true, 503: true, 504: true, 524: true}

type commonArgs struct {
	baseURL     string
	model       string
	size        string
	quality     string
	n           int
	outDir      string
	force       bool
	dryRun      bool
	maxAttempts int
	timeout     time.Duration
}

type apiResponse struct {
	Data []struct {
		B64JSON string `json:"b64_json"`
		URL     string `json:"url"`
	} `json:"data"`
}

type batchJob struct {
	Prompt  string `json:"prompt"`
	Model   string `json:"model,omitempty"`
	Size    string `json:"size,omitempty"`
	Quality string `json:"quality,omitempty"`
	N       int    `json:"n,omitempty"`
	Out     string `json:"out,omitempty"`
}

func endpoint(base, operation string) string {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	if !strings.HasSuffix(base, "/v1") {
		base += "/v1"
	}
	return base + "/images/" + operation
}

func validateCommon(args commonArgs) error {
	if strings.TrimSpace(args.baseURL) == "" {
		return errors.New("IMAGE_API_BASE_URL is not set; set it locally or pass --base-url, then retry")
	}
	if args.n < 1 || args.n > 10 {
		return errors.New("n must be from 1 to 10")
	}
	if args.maxAttempts < 1 {
		return errors.New("--max-attempts must be at least 1")
	}
	if args.timeout <= 0 {
		return errors.New("--timeout must be positive")
	}
	if args.quality != "low" && args.quality != "medium" && args.quality != "high" && args.quality != "auto" {
		return errors.New("quality must be low, medium, high, or auto")
	}
	if args.size != "auto" {
		parts := strings.Split(args.size, "x")
		if len(parts) != 2 {
			return errors.New("size must be 'auto' or WIDTHxHEIGHT")
		}
		w, e1 := strconv.Atoi(parts[0])
		h, e2 := strconv.Atoi(parts[1])
		if e1 != nil || e2 != nil || w < 1 || h < 1 {
			return errors.New("size must be 'auto' or WIDTHxHEIGHT")
		}
	}
	return nil
}

func promptValue(prompt, promptFile string) (string, error) {
	if (prompt == "") == (promptFile == "") {
		return "", errors.New("provide exactly one of --prompt or --prompt-file")
	}
	if promptFile != "" {
		data, err := os.ReadFile(promptFile)
		if err != nil {
			return "", fmt.Errorf("could not read prompt file: %w", err)
		}
		prompt = string(data)
	}
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return "", errors.New("prompt must not be empty")
	}
	return prompt, nil
}

func outputPaths(out, outDir, prompt string, n int) []string {
	if out == "" {
		sum := sha256.Sum256([]byte(prompt))
		out = "image-" + hex.EncodeToString(sum[:6]) + ".png"
	}
	if filepath.Dir(out) == "." {
		out = filepath.Join(outDir, out)
	}
	ext := filepath.Ext(out)
	if ext == "" {
		ext = ".png"
		out += ext
	}
	if n == 1 {
		return []string{out}
	}
	base := strings.TrimSuffix(out, ext)
	paths := make([]string, n)
	for i := range paths {
		paths[i] = fmt.Sprintf("%s-%d%s", base, i+1, ext)
	}
	return paths
}

func checkOutputs(paths []string, force bool) error {
	if force {
		return nil
	}
	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			return fmt.Errorf("refusing to overwrite existing file: %s", path)
		}
	}
	return nil
}

func apiKey() (string, error) {
	key := strings.TrimSpace(os.Getenv("IMAGE_API_KEY"))
	if key == "" {
		return "", errors.New("IMAGE_API_KEY is not set; set it locally, then retry")
	}
	return key, nil
}

func maxAttemptsFromEnv() (int, error) {
	raw := strings.TrimSpace(os.Getenv("IMAGE_API_MAX_ATTEMPTS"))
	if raw == "" {
		return defaultMaxAttempts, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 1 {
		return 0, errors.New("IMAGE_API_MAX_ATTEMPTS must be a positive integer")
	}
	return value, nil
}

func batchConcurrencyFromEnv() (int, error) {
	raw := strings.TrimSpace(os.Getenv("IMAGE_API_BATCH_CONCURRENCY"))
	if raw == "" {
		return defaultBatchConcurrency, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 1 {
		return 0, errors.New("IMAGE_API_BATCH_CONCURRENCY must be a positive integer")
	}
	return value, nil
}

func retryAfterDelay(value string) (time.Duration, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}
	if seconds, err := strconv.Atoi(value); err == nil {
		if seconds < 0 {
			return 0, false
		}
		delay := time.Duration(seconds) * time.Second
		if delay > retryMaxDelay {
			delay = retryMaxDelay
		}
		return delay, true
	}
	when, err := http.ParseTime(value)
	if err != nil {
		return 0, false
	}
	delay := time.Until(when)
	if delay < 0 {
		delay = 0
	}
	if delay > retryMaxDelay {
		delay = retryMaxDelay
	}
	return delay, true
}

func retryDelay(retryAfter string, attempt int) time.Duration {
	if delay, ok := retryAfterDelay(retryAfter); ok {
		return delay
	}
	shift := attempt - 1
	if shift > 6 {
		shift = 6
	}
	delay := retryBaseDelay * time.Duration(1<<shift)
	if delay > retryMaxDelay {
		delay = retryMaxDelay
	}
	return time.Duration(float64(delay) * (0.8 + rand.Float64()*0.4))
}

func doRequest(client *http.Client, makeRequest func() (*http.Request, error), maxAttempts int) ([]byte, error) {
	var last string
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		req, err := makeRequest()
		if err != nil {
			return nil, err
		}
		resp, err := client.Do(req)
		if err == nil {
			retryAfter := resp.Header.Get("Retry-After")
			body, readErr := io.ReadAll(resp.Body)
			resp.Body.Close()
			if readErr != nil {
				return nil, errors.New("could not read API response")
			}
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				return body, nil
			}
			last = fmt.Sprintf("API request failed with HTTP %d", resp.StatusCode)
			if !retryable[resp.StatusCode] || attempt == maxAttempts {
				return nil, errors.New(last)
			}
			delay := retryDelay(retryAfter, attempt)
			fmt.Fprintf(os.Stderr, "attempt %d/%d failed with HTTP %d; retrying in %s\n", attempt, maxAttempts, resp.StatusCode, delay.Round(time.Millisecond))
			time.Sleep(delay)
		} else {
			last = "API request failed: network error or timeout"
			if attempt == maxAttempts {
				return nil, errors.New(last)
			}
			delay := retryDelay("", attempt)
			fmt.Fprintf(os.Stderr, "attempt %d/%d failed with a network error or timeout; retrying in %s\n", attempt, maxAttempts, delay.Round(time.Millisecond))
			time.Sleep(delay)
		}
	}
	return nil, errors.New(last)
}

func decodeResponse(raw []byte, client *http.Client) ([][]byte, error) {
	var result apiResponse
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, errors.New("API returned invalid JSON")
	}
	if len(result.Data) == 0 {
		return nil, errors.New("API response contains no image data")
	}
	images := make([][]byte, 0, len(result.Data))
	for _, item := range result.Data {
		if item.B64JSON != "" {
			data, err := base64.StdEncoding.DecodeString(item.B64JSON)
			if err != nil {
				return nil, errors.New("API returned invalid base64 image data")
			}
			images = append(images, data)
		} else if item.URL != "" {
			if _, err := url.ParseRequestURI(item.URL); err != nil {
				return nil, errors.New("API returned an invalid image URL")
			}
			resp, err := client.Get(item.URL)
			if err != nil || resp.StatusCode < 200 || resp.StatusCode >= 300 {
				if resp != nil {
					resp.Body.Close()
				}
				return nil, errors.New("could not download image URL")
			}
			data, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil {
				return nil, errors.New("could not read downloaded image")
			}
			images = append(images, data)
		} else {
			return nil, errors.New("API image entry contains neither b64_json nor url")
		}
	}
	return images, nil
}

func saveImages(images [][]byte, paths []string) ([]string, error) {
	if len(images) != len(paths) {
		return nil, fmt.Errorf("API returned %d image(s), expected %d", len(images), len(paths))
	}
	abs := make([]string, len(paths))
	for i, path := range paths {
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return nil, fmt.Errorf("could not create output directory: %w", err)
		}
		if err := os.WriteFile(path, images[i], 0644); err != nil {
			return nil, fmt.Errorf("could not write output: %w", err)
		}
		abs[i], _ = filepath.Abs(path)
	}
	return abs, nil
}

func generate(prompt, out string, args commonArgs) (map[string]any, error) {
	if err := validateCommon(args); err != nil {
		return nil, err
	}
	paths := outputPaths(out, args.outDir, prompt, args.n)
	if err := checkOutputs(paths, args.force); err != nil {
		return nil, err
	}
	ep := endpoint(args.baseURL, "generations")
	payload := map[string]any{"model": args.model, "prompt": prompt, "size": args.size, "quality": args.quality, "n": args.n}
	if args.dryRun {
		return map[string]any{"dry_run": true, "endpoint": ep, "payload": payload, "outputs": paths}, nil
	}
	key, err := apiKey()
	if err != nil {
		return nil, err
	}
	body, _ := json.Marshal(payload)
	client := &http.Client{Timeout: args.timeout}
	raw, err := doRequest(client, func() (*http.Request, error) {
		req, err := http.NewRequest(http.MethodPost, ep, bytes.NewReader(body))
		if err == nil {
			req.Header.Set("Authorization", "Bearer "+key)
			req.Header.Set("Content-Type", "application/json")
		}
		return req, err
	}, args.maxAttempts)
	if err != nil {
		return nil, err
	}
	images, err := decodeResponse(raw, client)
	if err != nil {
		return nil, err
	}
	outputs, err := saveImages(images, paths)
	if err != nil {
		return nil, err
	}
	return map[string]any{"model": args.model, "size": args.size, "quality": args.quality, "outputs": outputs}, nil
}

func edit(prompt string, imagePaths []string, mask, out string, args commonArgs) (map[string]any, error) {
	if err := validateCommon(args); err != nil {
		return nil, err
	}
	for _, path := range append(append([]string{}, imagePaths...), mask) {
		if path == "" {
			continue
		}
		if info, err := os.Stat(path); err != nil || info.IsDir() {
			return nil, fmt.Errorf("input file not found: %s", path)
		}
	}
	paths := outputPaths(out, args.outDir, prompt, args.n)
	if err := checkOutputs(paths, args.force); err != nil {
		return nil, err
	}
	ep := endpoint(args.baseURL, "edits")
	fields := map[string]string{"model": args.model, "prompt": prompt, "size": args.size, "quality": args.quality, "n": strconv.Itoa(args.n)}
	if args.dryRun {
		return map[string]any{"dry_run": true, "endpoint": ep, "fields": fields, "images": imagePaths, "mask": mask, "outputs": paths}, nil
	}
	key, err := apiKey()
	if err != nil {
		return nil, err
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for name, value := range fields {
		_ = writer.WriteField(name, value)
	}
	for _, path := range imagePaths {
		part, err := writer.CreateFormFile("image", filepath.Base(path))
		if err != nil {
			return nil, err
		}
		file, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		_, err = io.Copy(part, file)
		file.Close()
		if err != nil {
			return nil, err
		}
	}
	if mask != "" {
		part, err := writer.CreateFormFile("mask", filepath.Base(mask))
		if err != nil {
			return nil, err
		}
		file, err := os.Open(mask)
		if err != nil {
			return nil, err
		}
		_, err = io.Copy(part, file)
		file.Close()
		if err != nil {
			return nil, err
		}
	}
	writer.Close()
	contentType := writer.FormDataContentType()
	data := append([]byte(nil), body.Bytes()...)
	client := &http.Client{Timeout: args.timeout}
	raw, err := doRequest(client, func() (*http.Request, error) {
		req, err := http.NewRequest(http.MethodPost, ep, bytes.NewReader(data))
		if err == nil {
			req.Header.Set("Authorization", "Bearer "+key)
			req.Header.Set("Content-Type", contentType)
		}
		return req, err
	}, args.maxAttempts)
	if err != nil {
		return nil, err
	}
	images, err := decodeResponse(raw, client)
	if err != nil {
		return nil, err
	}
	outputs, err := saveImages(images, paths)
	if err != nil {
		return nil, err
	}
	return map[string]any{"model": args.model, "size": args.size, "quality": args.quality, "outputs": outputs}, nil
}

func printJSON(value any) {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)
	_ = encoder.Encode(value)
}

func parseCommon(fs *flag.FlagSet, argv []string, args *commonArgs) error {
	var timeout float64
	maxAttempts, err := maxAttemptsFromEnv()
	if err != nil {
		return err
	}
	model := strings.TrimSpace(os.Getenv("IMAGE_API_MODEL"))
	if model == "" {
		model = defaultModel
	}
	fs.StringVar(&args.baseURL, "base-url", strings.TrimSpace(os.Getenv("IMAGE_API_BASE_URL")), "OpenAI-compatible API base URL")
	fs.StringVar(&args.model, "model", model, "image model")
	fs.StringVar(&args.size, "size", defaultSize, "auto or WIDTHxHEIGHT")
	fs.StringVar(&args.quality, "quality", defaultQuality, "low, medium, high, or auto")
	fs.IntVar(&args.n, "n", 1, "number of variants (1-10)")
	fs.StringVar(&args.outDir, "out-dir", defaultOutDir, "default output directory")
	fs.BoolVar(&args.force, "force", false, "overwrite existing outputs")
	fs.BoolVar(&args.dryRun, "dry-run", false, "validate without network access")
	fs.IntVar(&args.maxAttempts, "max-attempts", maxAttempts, "total request attempts (at least 1)")
	fs.Float64Var(&timeout, "timeout", 150, "request timeout in seconds")
	if err := fs.Parse(argv); err != nil {
		return err
	}
	args.timeout = time.Duration(timeout * float64(time.Second))
	return nil
}

type stringList []string

func (s *stringList) String() string { return strings.Join(*s, ",") }
func (s *stringList) Set(value string) error {
	*s = append(*s, value)
	return nil
}

func runGenerate(argv []string) error {
	fs := flag.NewFlagSet("generate", flag.ContinueOnError)
	var prompt, promptFile, out string
	fs.StringVar(&prompt, "prompt", "", "prompt text")
	fs.StringVar(&promptFile, "prompt-file", "", "UTF-8 prompt file")
	fs.StringVar(&out, "out", "", "output path")
	var args commonArgs
	if err := parseCommon(fs, argv, &args); err != nil {
		return err
	}
	finalPrompt, err := promptValue(prompt, promptFile)
	if err != nil {
		return err
	}
	result, err := generate(finalPrompt, out, args)
	if err == nil {
		printJSON(result)
	}
	return err
}

func runEdit(argv []string) error {
	fs := flag.NewFlagSet("edit", flag.ContinueOnError)
	var images stringList
	var prompt, promptFile, mask, out string
	fs.Var(&images, "image", "input image; repeat for multiple images")
	fs.StringVar(&mask, "mask", "", "optional PNG mask")
	fs.StringVar(&prompt, "prompt", "", "prompt text")
	fs.StringVar(&promptFile, "prompt-file", "", "UTF-8 prompt file")
	fs.StringVar(&out, "out", "", "output path")
	var args commonArgs
	if err := parseCommon(fs, argv, &args); err != nil {
		return err
	}
	if len(images) == 0 {
		return errors.New("provide at least one --image")
	}
	finalPrompt, err := promptValue(prompt, promptFile)
	if err != nil {
		return err
	}
	result, err := edit(finalPrompt, images, mask, out, args)
	if err == nil {
		printJSON(result)
	}
	return err
}

func runBatch(argv []string) error {
	fs := flag.NewFlagSet("generate-batch", flag.ContinueOnError)
	var input string
	concurrency, err := batchConcurrencyFromEnv()
	if err != nil {
		return err
	}
	var failFast bool
	fs.StringVar(&input, "input", "", "JSONL input path")
	fs.IntVar(&concurrency, "concurrency", concurrency, "parallel jobs")
	fs.BoolVar(&failFast, "fail-fast", false, "stop scheduling after a failure")
	var args commonArgs
	if err := parseCommon(fs, argv, &args); err != nil {
		return err
	}
	if input == "" || concurrency < 1 {
		return errors.New("--input is required and --concurrency must be at least 1")
	}
	file, err := os.Open(input)
	if err != nil {
		return fmt.Errorf("could not read batch input: %w", err)
	}
	defer file.Close()
	var jobs []batchJob
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		if strings.TrimSpace(scanner.Text()) == "" {
			continue
		}
		var job batchJob
		if err := json.Unmarshal(scanner.Bytes(), &job); err != nil || strings.TrimSpace(job.Prompt) == "" {
			return errors.New("each JSONL line must contain a non-empty prompt")
		}
		jobs = append(jobs, job)
	}
	if err := scanner.Err(); err != nil || len(jobs) == 0 {
		return errors.New("batch input contains no valid jobs")
	}
	type result struct {
		Index int            `json:"index"`
		OK    bool           `json:"ok"`
		Data  map[string]any `json:"data,omitempty"`
		Error string         `json:"error,omitempty"`
	}
	results := make([]result, len(jobs))
	queue := make(chan int)
	var wg sync.WaitGroup
	var failed bool
	var mu sync.Mutex
	for range concurrency {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for index := range queue {
				job := jobs[index]
				jobArgs := args
				if job.Model != "" {
					jobArgs.model = job.Model
				}
				if job.Size != "" {
					jobArgs.size = job.Size
				}
				if job.Quality != "" {
					jobArgs.quality = job.Quality
				}
				if job.N != 0 {
					jobArgs.n = job.N
				}
				data, err := generate(strings.TrimSpace(job.Prompt), job.Out, jobArgs)
				results[index] = result{Index: index + 1, OK: err == nil, Data: data}
				if err != nil {
					results[index].Error = err.Error()
					mu.Lock()
					failed = true
					mu.Unlock()
				}
			}
		}()
	}
	for index := range jobs {
		mu.Lock()
		stop := failFast && failed
		mu.Unlock()
		if stop {
			break
		}
		queue <- index
	}
	close(queue)
	wg.Wait()
	succeeded, failures := 0, 0
	for _, item := range results {
		if item.Index == 0 {
			continue
		}
		if item.OK {
			succeeded++
		} else {
			failures++
		}
	}
	printJSON(map[string]any{"concurrency": concurrency, "jobs": results, "succeeded": succeeded, "failed": failures})
	if failures > 0 {
		return errors.New("one or more batch jobs failed")
	}
	return nil
}

func usage() {
	fmt.Fprintln(os.Stderr, "Usage: img-gen <generate|generate-batch|edit> [options]")
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "generate":
		err = runGenerate(os.Args[2:])
	case "generate-batch":
		err = runBatch(os.Args[2:])
	case "edit":
		err = runEdit(os.Args[2:])
	case "--help", "-h", "help":
		usage()
		return
	default:
		err = fmt.Errorf("unknown command: %s", os.Args[1])
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(2)
	}
}
