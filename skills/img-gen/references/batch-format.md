# Batch JSONL format

The `batch` command reads one JSON object per non-empty line. Every job is a text generation task and must explicitly declare `operation: "generate"`.

```jsonl
{"operation":"generate","prompt":"A blue ceramic mug on white","out":"mug.png"}
{"operation":"generate","prompt":"A red paper kite in a clear sky","size":"1536x1024","quality":"low","out":"kite.png"}
```

## Required fields

- `operation` must be exactly `generate`; missing or unknown operations fail validation.
- `prompt` must be non-empty.
- `out` must be non-empty.

Supported optional fields are `model`, `size`, and `quality`. Each job requests exactly one image, preserving the single-image contract of the `generate` command.

## Output paths and preflight

Relative `out` paths are resolved under the command's `--out-dir`; absolute paths remain absolute. The CLI resolves every output, rejects duplicate resolved paths, checks output parent paths, and protects existing files before making any network request. Use unique output names. Pass `--force` only when overwriting is explicitly authorized.

## Concurrency and failures

Concurrency is selected by `--concurrency`, then `IMAGE_API_BATCH_CONCURRENCY`, then the default `5`. The bounded worker pool replaces a completed job immediately. By default, a failed job does not stop other jobs; successful files are kept, the JSON summary remains in input order, and the process exits nonzero if any job fails. `--fail-fast` stops scheduling jobs that have not started while allowing already-started jobs to finish.

Run a batch with:

```bash
"<img-gen>" batch \
  --input "tmp/imagegen/jobs.jsonl" \
  --out-dir "output/imagegen"
```
