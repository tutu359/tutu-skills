# Batch JSONL format

The `batch` command reads one JSON object per non-empty line. Every job must explicitly declare `operation` as either `generate` or `edit`; the CLI never infers an operation from the other fields.

```jsonl
{"operation":"generate","prompt":"A blue ceramic mug on white","out":"mug.png"}
{"operation":"edit","prompt":"Keep both products unchanged and combine them on one shelf","image":["inputs/left.png","inputs/right.png"],"mask":"inputs/shelf-mask.png","out":"shelf.png"}
```

## Common fields

- `operation` is required and must be exactly `generate` or `edit`. Missing or unknown operations fail before any network request.
- `prompt` is required and must be non-empty. The prompt is sent unchanged; the CLI does not interpret image roles or rewrite it.
- `out` is required and must be unique across both generate and edit jobs.
- `model`, `size`, and `quality` are optional per-job overrides. Each job requests exactly one image, preserving the single-image contract of the individual commands.

## Generate jobs

A generate job has no input images:

```jsonl
{"operation":"generate","prompt":"A red paper kite in a clear sky","size":"1536x1024","quality":"low","out":"kite.png"}
```

`image` and `mask` must not appear at all on a generate job, including as empty values.

## Edit jobs

An edit job must contain at least one image and may contain one mask:

```jsonl
{"operation":"edit","prompt":"Replace only the background","image":["product.png"],"out":"product-edited.png"}
{"operation":"edit","prompt":"Blend the references into one composition","image":["subject.png","style.png"],"mask":"region.png","out":"composite.png"}
```

- `image` is an array with one or more paths. Images are uploaded in exactly the array order. The CLI does not assign roles to them.
- `mask` is one optional path. It is uploaded after all `image` parts.
- Edit input files must exist and must not be directories.

## Path resolution and preflight

Relative `image` and `mask` paths are resolved from the directory containing the batch JSONL file. Absolute input paths remain absolute. Relative `out` paths are resolved under the command's `--out-dir`; absolute output paths remain absolute. Output extensions are handled as for the individual commands.

The CLI validates every operation, prompt, input file, output path, output conflict, and duplicate resolved output before starting any network request. Use unique output names. Pass `--force` only when overwriting is explicitly authorized.

## Concurrency and failures

Concurrency is selected by `--concurrency`, then `IMAGE_API_BATCH_CONCURRENCY`, then the default `5`. The bounded worker pool replaces a completed job immediately. By default, a failed job does not stop other jobs; successful files are kept, the JSON summary remains in input order, and the process exits nonzero if any job fails. `--fail-fast` stops scheduling jobs that have not started while allowing already-started jobs to finish. The summary includes each job's `operation` and either its successful `outputs` or an `error` reason.

Run a batch with:

```bash
"<img-gen>" batch \
  --input "tmp/imagegen/jobs.jsonl" \
  --out-dir "output/imagegen"
```
