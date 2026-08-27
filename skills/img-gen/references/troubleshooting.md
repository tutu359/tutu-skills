# Configuration and troubleshooting

This is the first reference after every failed `generate`, `edit`, or `batch` command. Use it before a Provider reference. Normal successful tasks do not read this file.

## Configuration

- A normal image task is attempted before troubleshooting or initialization is suggested; do not preflight a Provider, endpoint, environment variable, reference image, or Model capability.
- If the CLI reports missing or unusable Provider Configuration, run `<img-gen> init` after the failed task. It creates the user-level JSON template at the operating-system user configuration directory. Fill the Provider credentials locally and retry the original task.
- Initialization does not send a network request, does not read legacy configuration, and does not print credential values. Never ask a user to paste an API Key into chat or include one in a diagnostic report.
- Require an explicit `--provider` or `defaultProvider` in the user-level JSON configuration.
- Require the selected Provider's `baseURL`, `apiKey`, and `model` fields. Provider identity is never inferred from a credential, endpoint, or Model.
- Legacy `IMAGE_API_*` environment variables are ignored.
- Default size and quality to `auto`.

## Execution permission

- On macOS, only after a permission-denied error, run `chmod +x bin/img-gen bin/img-gen-darwin-*` and retry once. Do not run `chmod` preemptively.
- On Windows, use `bin\\img-gen.cmd`; it selects `bin/img-gen-windows-amd64.exe` or `bin/img-gen-windows-arm64.exe` from the system architecture variables.

## Network, timeout, and retry

- The CLI retries network failures, timeouts, and HTTP `5xx` failures with bounded backoff. It honors a valid `Retry-After` header, capped at 30 seconds.
- Do not retry any `4xx` response, including `429`, authentication, validation, policy, or ordinary request errors.
- On a repeated timeout, check the selected Provider's Base URL and network access, then try lower quality, a square size, fewer concurrent batch jobs, or a later retry. Do not silently change Provider or Model.
- Never expose authorization headers, API Keys, or full API response bodies. Error output contains only a safe status or message.

## Batch rules

- Batch uses one global worker pool selected by `--concurrency`; it has no Provider-specific queues, rate limits, or automatic fallback.
- The default continues scheduling jobs after a failure, keeps successful files, preserves input order in the JSON summary, and exits nonzero if any job fails.
- `--fail-fast` stops scheduling jobs that have not started while already-started jobs finish. It does not remove successful outputs.
- Resolve the reported job index, operation, output path, and safe error first. Do not discard successful outputs or visually inspect them as an automatic retry.
