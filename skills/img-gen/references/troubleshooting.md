# Configuration and troubleshooting

Read this file only after a command fails or when the user asks about configuration behavior.

## Configuration

- Require `IMAGE_API_BASE_URL` or `--base-url`.
- Require `IMAGE_API_KEY` for network requests. Ask the user to configure it locally; never ask them to paste it into chat.
- Resolve the model from `--model`, then `IMAGE_API_MODEL`, then `gpt-image-2`.
- Resolve batch concurrency from `--concurrency`, then `IMAGE_API_BATCH_CONCURRENCY`, then `5`.
- Resolve total request attempts from `--max-attempts`, then `IMAGE_API_MAX_ATTEMPTS`, then `3`.
- Default size and quality to `auto`.

## Failures

- On macOS, after a permission-denied error, run `chmod +x bin/img-gen bin/img-gen-darwin-*` and retry once. Do not run it preemptively.
- On Windows, use `bin\img-gen.cmd`; it selects `img-gen-windows-amd64.exe` or `img-gen-windows-arm64.exe` from the system architecture variables.
- The CLI retries network timeouts and HTTP 429/500/502/503/504/524 failures with bounded backoff. It honors a valid `Retry-After` header, capped at 30 seconds.
- Do not retry authentication, validation, or ordinary 4xx errors.
- On repeated timeout, try lower quality, a square size, fewer concurrent jobs, or a later retry.
- Never expose authorization headers, API keys, or full API response bodies.
