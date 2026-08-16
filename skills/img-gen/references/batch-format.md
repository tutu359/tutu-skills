# Batch JSONL format

Write one JSON object per line. Each job requires `prompt` and may override generation or output settings.

```jsonl
{"prompt":"A blue ceramic mug on white","out":"mug.png"}
{"prompt":"A red paper kite in a clear sky","size":"1536x1024","quality":"low","out":"kite.png"}
```

Supported fields are `prompt`, `size`, `quality`, `out`, and `model`. Relative `out` paths are resolved under `--out-dir`. Blank lines are ignored. Every job requests and expects exactly one image; quantity fields are not supported.

Batch concurrency is controlled outside the JSONL file: `--concurrency` overrides `IMAGE_API_BATCH_CONCURRENCY`, which defaults to `5`.

Use unique output names. The batch command prints a JSON summary and exits nonzero if any job fails.
