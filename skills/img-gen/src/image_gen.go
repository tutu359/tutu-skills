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
	"time"
)

const (
	defaultSize             = "auto"
	defaultQuality          = "auto"
	defaultOutDir           = "output/imagegen"
	defaultMaxAttempts      = 3
	defaultBatchConcurrency = 5
	retryBaseDelay          = 750 * time.Millisecond
	retryMaxDelay           = 30 * time.Second
)

var retryable = map[int]bool{500: true, 502: true, 503: true, 504: true, 524: true}

type commonArgs struct {
	provider        string
	baseURL         string
	baseURLOverride string
	apiKey          string
	model           string
	size            string
	quality         string
	outDir          string
	force           bool
	dryRun          bool
	maxAttempts     int
	timeout         time.Duration
}

type apiResponse struct {
	Data []struct {
		B64JSON string `json:"b64_json"`
		URL     string `json:"url"`
	} `json:"data"`
}

type batchJob struct {
	Operation string   `json:"operation"`
	Provider  string   `json:"provider,omitempty"`
	Prompt    string   `json:"prompt"`
	Images    []string `json:"image,omitempty"`
	Mask      string   `json:"mask,omitempty"`
	imageSet  bool
	maskSet   bool
	Model     string `json:"model,omitempty"`
	Size      string `json:"size,omitempty"`
	Quality   string `json:"quality,omitempty"`
	Out       string `json:"out"`
}

func endpoint(base, operation string) string {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	if !strings.HasSuffix(base, "/v1") {
		base += "/v1"
	}
	return base + "/images/" + operation
}

type providerConfig struct {
	BaseURL string `json:"baseURL"`
	APIKey  string `json:"apiKey"`
	Model   string `json:"model"`
}

type userConfig struct {
	DefaultProvider string                    `json:"defaultProvider"`
	Providers       map[string]providerConfig `json:"providers"`
}

func configFilePath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("could not locate the user configuration directory: %w", err)
	}
	return filepath.Join(dir, "tutu-skills", "img-gen", "config.json"), nil
}

func selectProvider(args commonArgs, explicit string) (commonArgs, error) {
	path, err := configFilePath()
	if err != nil {
		return args, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return args, fmt.Errorf("img-gen Provider Configuration is missing at %s; create it with defaultProvider and providers.openai.baseURL, apiKey, and model, then retry", path)
		}
		return args, fmt.Errorf("could not read img-gen Provider Configuration %s: %w", path, err)
	}
	var config userConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return args, fmt.Errorf("img-gen Provider Configuration %s is invalid JSON: %w", path, err)
	}
	provider := strings.TrimSpace(explicit)
	if provider == "" {
		provider = strings.TrimSpace(config.DefaultProvider)
	}
	if provider == "" {
		return args, fmt.Errorf("no Provider selected; pass --provider or set defaultProvider in %s", path)
	}
	if provider != "openai" {
		return args, fmt.Errorf("unsupported Provider %q; supported Provider: openai", provider)
	}
	selected, ok := config.Providers[provider]
	if !ok {
		return args, fmt.Errorf("Provider Configuration for %q is missing in %s", provider, path)
	}
	if strings.TrimSpace(selected.BaseURL) == "" || strings.TrimSpace(selected.APIKey) == "" || strings.TrimSpace(selected.Model) == "" {
		return args, fmt.Errorf("Provider Configuration for %q must include non-empty baseURL, apiKey, and model in %s", provider, path)
	}
	args.provider = provider
	args.baseURL = strings.TrimSpace(selected.BaseURL)
	if strings.TrimSpace(args.baseURLOverride) != "" {
		args.baseURL = strings.TrimSpace(args.baseURLOverride)
	}
	args.apiKey = strings.TrimSpace(selected.APIKey)
	args.model = strings.TrimSpace(selected.Model)
	return args, nil
}

func validateCommon(args commonArgs) error {
	if strings.TrimSpace(args.provider) == "" {
		return errors.New("Provider Selection is required")
	}
	if strings.TrimSpace(args.baseURL) == "" {
		return errors.New("OpenAI Provider Configuration baseURL is empty")
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

func outputPaths(out, outDir, prompt string) []string {
	if out == "" {
		sum := sha256.Sum256([]byte(prompt))
		out = "image-" + hex.EncodeToString(sum[:6]) + ".png"
	}
	if filepath.Dir(out) == "." {
		out = filepath.Join(outDir, out)
	}
	ext := filepath.Ext(out)
	if ext == "" {
		out += ".png"
	}
	return []string{out}
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
	paths := outputPaths(out, args.outDir, prompt)
	if err := checkOutputs(paths, args.force); err != nil {
		return nil, err
	}
	ep := endpoint(args.baseURL, "generations")
	payload := map[string]any{"model": args.model, "prompt": prompt, "size": args.size, "quality": args.quality, "n": 1}
	if args.dryRun {
		return map[string]any{"dry_run": true, "provider": args.provider, "model": args.model, "endpoint": ep, "payload": payload, "outputs": paths}, nil
	}
	key := strings.TrimSpace(args.apiKey)
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
	return map[string]any{"provider": args.provider, "model": args.model, "size": args.size, "quality": args.quality, "outputs": outputs}, nil
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
	paths := outputPaths(out, args.outDir, prompt)
	if err := checkOutputs(paths, args.force); err != nil {
		return nil, err
	}
	ep := endpoint(args.baseURL, "edits")
	fields := map[string]string{"model": args.model, "prompt": prompt, "size": args.size, "quality": args.quality, "n": "1"}
	if args.dryRun {
		return map[string]any{"dry_run": true, "provider": args.provider, "model": args.model, "endpoint": ep, "fields": fields, "images": imagePaths, "mask": mask, "outputs": paths}, nil
	}
	key := strings.TrimSpace(args.apiKey)
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
	return map[string]any{"provider": args.provider, "model": args.model, "size": args.size, "quality": args.quality, "outputs": outputs}, nil
}

func writeJSON(w io.Writer, value any) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return fmt.Errorf("could not write JSON output: %w", err)
	}
	return nil
}

func parseCommon(fs *flag.FlagSet, argv []string, args *commonArgs) error {
	var timeout float64
	maxAttempts := defaultMaxAttempts
	fs.StringVar(&args.provider, "provider", "", "Provider Selection (for example, openai)")
	// --base-url is retained as an explicit endpoint override for local gateways;
	// it never selects a Provider and is never populated from environment variables.
	fs.StringVar(&args.baseURLOverride, "base-url", "", "explicit OpenAI endpoint override")
	fs.StringVar(&args.size, "size", defaultSize, "auto or WIDTHxHEIGHT")
	fs.StringVar(&args.quality, "quality", defaultQuality, "low, medium, high, or auto")
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

func runGenerate(argv []string, output io.Writer) error {
	fs := flag.NewFlagSet("generate", flag.ContinueOnError)
	var prompt, promptFile, out string
	fs.StringVar(&prompt, "prompt", "", "prompt text")
	fs.StringVar(&promptFile, "prompt-file", "", "UTF-8 prompt file")
	fs.StringVar(&out, "out", "", "output path")
	var args commonArgs
	if err := parseCommon(fs, argv, &args); err != nil {
		return err
	}
	var err error
	args, err = selectProvider(args, args.provider)
	if err != nil {
		return err
	}
	finalPrompt, err := promptValue(prompt, promptFile)
	if err != nil {
		return err
	}
	result, err := generate(finalPrompt, out, args)
	if err != nil {
		return err
	}
	return writeJSON(output, result)
}

func runEdit(argv []string, output io.Writer) error {
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
	var err error
	args, err = selectProvider(args, args.provider)
	if err != nil {
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
	if err != nil {
		return err
	}
	return writeJSON(output, result)
}

type batchResult struct {
	Index     int            `json:"index"`
	Operation string         `json:"operation"`
	Provider  string         `json:"provider,omitempty"`
	Model     string         `json:"model,omitempty"`
	OK        bool           `json:"ok"`
	Outputs   []string       `json:"outputs,omitempty"`
	Data      map[string]any `json:"data,omitempty"`
	Error     string         `json:"error,omitempty"`
}

func batchJobArgs(job batchJob, args commonArgs) commonArgs {
	if strings.TrimSpace(job.Model) != "" {
		args.model = strings.TrimSpace(job.Model)
	}
	if strings.TrimSpace(job.Size) != "" {
		args.size = strings.TrimSpace(job.Size)
	}
	if strings.TrimSpace(job.Quality) != "" {
		args.quality = strings.TrimSpace(job.Quality)
	}
	return args
}

func batchInputPath(raw, batchDir string) (string, error) {
	path := strings.TrimSpace(raw)
	if path == "" {
		return "", errors.New("input path must not be empty")
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(batchDir, path)
	}
	path = filepath.Clean(path)
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("input file not found: %s", path)
		}
		return "", fmt.Errorf("could not inspect input file %s: %w", path, err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("input path is a directory: %s", path)
	}
	return path, nil
}

func batchOutputPath(job batchJob, args commonArgs) (string, error) {
	out := strings.TrimSpace(job.Out)
	if out == "" {
		return "", errors.New("out must not be empty")
	}
	if !filepath.IsAbs(out) {
		out = filepath.Join(args.outDir, out)
	}
	paths := outputPaths(out, args.outDir, strings.TrimSpace(job.Prompt))
	if len(paths) != 1 {
		return "", errors.New("batch jobs must produce exactly one output")
	}
	return filepath.Clean(paths[0]), nil
}

func validateBatchOutput(path string, force bool) error {
	if _, err := filepath.Abs(path); err != nil {
		return fmt.Errorf("invalid output path %q: %w", path, err)
	}
	if info, err := os.Lstat(path); err == nil {
		if info.IsDir() {
			return fmt.Errorf("output path is a directory: %s", path)
		}
		if !force {
			return fmt.Errorf("refusing to overwrite existing file: %s", path)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("could not inspect output path %s: %w", path, err)
	}
	parent := filepath.Dir(path)
	if info, err := os.Stat(parent); err == nil {
		if !info.IsDir() {
			return fmt.Errorf("output parent is not a directory: %s", parent)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("could not inspect output directory %s: %w", parent, err)
	}
	return nil
}

func runBatch(argv []string, output io.Writer) error {
	fs := flag.NewFlagSet("batch", flag.ContinueOnError)
	var input string
	var concurrency int
	var failFast bool
	fs.StringVar(&input, "input", "", "JSONL input path")
	fs.IntVar(&concurrency, "concurrency", defaultBatchConcurrency, "parallel jobs")
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
	scanner.Buffer(make([]byte, 64*1024), 10*1024*1024)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		if strings.TrimSpace(scanner.Text()) == "" {
			continue
		}
		var job batchJob
		if err := json.Unmarshal(scanner.Bytes(), &job); err != nil {
			return fmt.Errorf("batch line %d is invalid JSON: %w", lineNumber, err)
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(scanner.Bytes(), &fields); err != nil {
			return fmt.Errorf("batch line %d is invalid JSON: %w", lineNumber, err)
		}
		_, job.imageSet = fields["image"]
		_, job.maskSet = fields["mask"]
		operation := job.Operation
		if operation == "" {
			return fmt.Errorf("batch line %d must declare operation as generate or edit", lineNumber)
		}
		if operation != "generate" && operation != "edit" {
			return fmt.Errorf("batch line %d has unknown operation %q; expected generate or edit", lineNumber, operation)
		}
		job.Operation = operation
		if strings.TrimSpace(job.Prompt) == "" {
			return fmt.Errorf("batch line %d prompt must not be empty", lineNumber)
		}
		if strings.TrimSpace(job.Out) == "" {
			return fmt.Errorf("batch line %d out must not be empty", lineNumber)
		}
		if operation == "generate" {
			if job.imageSet {
				return fmt.Errorf("batch line %d generate jobs must not include image", lineNumber)
			}
			if job.maskSet {
				return fmt.Errorf("batch line %d generate jobs must not include mask", lineNumber)
			}
		} else {
			if len(job.Images) == 0 {
				return fmt.Errorf("batch line %d edit jobs must provide at least one image", lineNumber)
			}
			if job.maskSet && strings.TrimSpace(job.Mask) == "" {
				return fmt.Errorf("batch line %d edit mask must not be empty", lineNumber)
			}
		}
		jobs = append(jobs, job)
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("could not read batch input: %w", err)
	}
	if len(jobs) == 0 {
		return errors.New("batch input contains no valid jobs")
	}

	// Resolve and validate every input and output before starting any worker.
	// This keeps malformed jobs and missing files from causing partial network work.
	batchPath, err := filepath.Abs(input)
	if err != nil {
		return fmt.Errorf("could not resolve batch input path: %w", err)
	}
	batchDir := filepath.Dir(batchPath)
	resolved := make([]string, len(jobs))
	resolvedArgs := make([]commonArgs, len(jobs))
	seen := make(map[string]int, len(jobs))
	for i := range jobs {
		job := &jobs[i]
		// Resolve the Provider Configuration first: the job-level provider
		// overrides the command's --provider, which overrides defaultProvider.
		// Job-level model, size, and quality then apply within the selected
		// Provider without changing the Provider Selection.
		explicit := strings.TrimSpace(job.Provider)
		if explicit == "" {
			explicit = args.provider
		}
		jobArgs, err := selectProvider(args, explicit)
		if err != nil {
			return fmt.Errorf("batch line %d: %w", i+1, err)
		}
		jobArgs = batchJobArgs(*job, jobArgs)
		if err := validateCommon(jobArgs); err != nil {
			return fmt.Errorf("batch line %d is invalid: %w", i+1, err)
		}
		if job.Operation == "edit" {
			for imageIndex, image := range job.Images {
				resolvedImage, err := batchInputPath(image, batchDir)
				if err != nil {
					return fmt.Errorf("batch line %d image %d: %w", i+1, imageIndex+1, err)
				}
				job.Images[imageIndex] = resolvedImage
			}
			if strings.TrimSpace(job.Mask) != "" {
				resolvedMask, err := batchInputPath(job.Mask, batchDir)
				if err != nil {
					return fmt.Errorf("batch line %d mask: %w", i+1, err)
				}
				job.Mask = resolvedMask
			}
		}
		path, err := batchOutputPath(*job, args)
		if err != nil {
			return fmt.Errorf("batch line %d: %w", i+1, err)
		}
		absolute, err := filepath.Abs(path)
		if err != nil {
			return fmt.Errorf("batch line %d has invalid output path: %w", i+1, err)
		}
		absolute = filepath.Clean(absolute)
		if previous, exists := seen[absolute]; exists {
			return fmt.Errorf("batch lines %d and %d resolve to the same output: %s", previous, i+1, absolute)
		}
		seen[absolute] = i + 1
		if err := validateBatchOutput(path, args.force); err != nil {
			return fmt.Errorf("batch line %d: %w", i+1, err)
		}
		resolved[i] = path
		resolvedArgs[i] = jobArgs
	}

	results := make([]batchResult, len(jobs))
	done := make(chan struct {
		index int
		data  map[string]any
		err   error
	})
	next, active := 0, 0
	launch := func(index int) {
		active++
		go func() {
			job := jobs[index]
			jobArgs := resolvedArgs[index]
			var data map[string]any
			var err error
			if job.Operation == "edit" {
				data, err = edit(job.Prompt, job.Images, job.Mask, resolved[index], jobArgs)
			} else {
				data, err = generate(job.Prompt, resolved[index], jobArgs)
			}
			done <- struct {
				index int
				data  map[string]any
				err   error
			}{index: index, data: data, err: err}
		}()
	}
	for active < concurrency && next < len(jobs) {
		launch(next)
		next++
	}
	failed := false
	for active > 0 {
		finished := <-done
		active--
		result := batchResult{Index: finished.index + 1, Operation: jobs[finished.index].Operation, Provider: resolvedArgs[finished.index].provider, Model: resolvedArgs[finished.index].model, OK: finished.err == nil, Data: finished.data}
		if finished.err != nil {
			result.Error = finished.err.Error()
			failed = true
		} else if outputs, ok := finished.data["outputs"].([]string); ok {
			result.Outputs = outputs
		}
		results[finished.index] = result
		if (!failFast || !failed) && next < len(jobs) {
			launch(next)
			next++
		}
	}
	if failFast && failed {
		for index := next; index < len(jobs); index++ {
			results[index] = batchResult{Index: index + 1, Operation: jobs[index].Operation, Provider: resolvedArgs[index].provider, Model: resolvedArgs[index].model, Error: "not started because fail-fast was triggered"}
		}
	}

	succeeded, failures := 0, 0
	for _, item := range results {
		if item.OK {
			succeeded++
		} else {
			failures++
		}
	}
	if err := writeJSON(output, map[string]any{
		"concurrency": concurrency,
		"jobs":        results,
		"succeeded":   succeeded,
		"failed":      failures,
	}); err != nil {
		return err
	}
	if failures > 0 {
		return errors.New("one or more batch jobs failed")
	}
	return nil
}

func usage() {
	fmt.Fprintln(os.Stderr, "Usage: img-gen <generate|edit|batch> [options]")
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "generate":
		err = runGenerate(os.Args[2:], os.Stdout)
	case "edit":
		err = runEdit(os.Args[2:], os.Stdout)
	case "batch":
		err = runBatch(os.Args[2:], os.Stdout)
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
