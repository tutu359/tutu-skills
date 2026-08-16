---
name: img-gen
description: Generate and edit raster images through a configurable OpenAI-compatible Image API. Use when the user asks for one or many images, illustrations, product shots, covers, website assets, visual variants, background replacements, object changes, compositing, or other image edits through a separately configured API provider.
---

# img-gen

Generate and edit images with the bundled native macOS and Windows CLI. No Python, Node.js, Go, or package installation is required during normal use.

## Control plane and execution plane

The Skill is the **control plane**. It faithfully turns the user's request into prompts, one-image tasks, output names, and a single invocation plan. The CLI is the **execution plane**. It owns argument validation, batch preflight, path resolution, API requests, bounded concurrency, retries, file writes, the JSON summary, and exit status.

Keep the normal path fast. Do not search the workspace, inspect attachments, probe configuration, compile, run a dry run, or read reference documents before the first requested operation. Read a reference document only when its details are needed to resolve an unfamiliar format, error, or troubleshooting question.

## Fast path

For every normal `generate`, `edit`, or `batch` request:

1. Use only the prompt, attachments, and settings the user explicitly provides.
2. Select the launcher from the current operating system, then run it directly:
   - macOS: `bin/img-gen`
   - Windows: `bin\img-gen.cmd`
   Both launchers select the correct CPU architecture.
3. Let the CLI validate API configuration, output paths, and arguments. Do not pre-check environment variables.
4. On macOS, do not run `chmod` preemptively. Repair execution permission only after a permission-denied error, then retry once.
5. In the same session, reuse established output conventions and request settings unless the user changes them.

In the examples below, replace `<img-gen>` with the launcher for the current operating system.

## Multi-image orchestration

A request that asks for more than one output is one **Generation Set**. Create one JSONL job per output and invoke `<img-gen> batch` exactly once for the set. Do not start one shell command per image, do not start multiple independent shells, and do not use a single request's image quantity option. Every task produces exactly one image.

Use unique, deterministic, user-appropriate output names. The names may identify the set and sequence, but changing a filename does not authorize changing a prompt.

### No requested differences means exact prompts

If the user only asks for multiple copies and does not ask for differences, repeat the prompt **exactly** in every `generate` job. Do not add numbering, variation language, or other text to the prompt. Make only the `out` values unique.

```jsonl
{"operation":"generate","prompt":"A small blue nebula in a glass bottle, studio product photo","out":"nebula-01.png"}
{"operation":"generate","prompt":"A small blue nebula in a glass bottle, studio product photo","out":"nebula-02.png"}
{"operation":"generate","prompt":"A small blue nebula in a glass bottle, studio product photo","out":"nebula-03.png"}
```

The prompts are intentionally identical. The Skill guarantees intentional task-prompt differences only when the user authorizes a variation; it does not promise pixel-level differences between generated results.

### Authorized variations only

Create a derived prompt only for a dimension the user explicitly authorizes. Preserve every other dimension exactly, including subject, action, composition, camera treatment, lighting, palette, text, constraints, and avoid-items. A request such as “make a few variations” is not enough by itself: the user must authorize a dimension such as style, pose, or scene. Once a dimension is authorized, choose concrete values when the user leaves them open, but do not silently change every visual dimension.

Supported variation dimensions can be combined when the user authorizes them:

- **Performance or rendering variants**: pose, gesture, expression, camera angle, framing, lighting, or rendering treatment.
- **World expansion**: different locations or corners of the same established world while preserving its authorized canon.
- **Subject dimensions**: person, character, object, scene, background, environment, palette, or style.

When an authorized dimension has no concrete value, choose suitable values and generate directly. Do not ask one question per image or add a confirmation step. Do not change an unauthorized dimension while choosing those values.

Every derived prompt must be self-contained. Repeat the complete common core, all stable dimensions, the one authorized change for that task, and all user constraints in that prompt. Never rely on “same as above”, “as before”, task order, or another job for meaning.

For example, if the user authorizes different styles but not different subjects, each prompt must restate the subject and stable scene:

```jsonl
{"operation":"generate","prompt":"A red paper kite above a coastal cliff at sunrise; landscape composition, clear sky, no text; render in watercolor","out":"kite-watercolor.png"}
{"operation":"generate","prompt":"A red paper kite above a coastal cliff at sunrise; landscape composition, clear sky, no text; render in charcoal","out":"kite-charcoal.png"}
```

If the user authorizes different corners of one world, repeat the world core and constraints in every prompt:

```jsonl
{"operation":"generate","prompt":"The same floating city of brass bridges, blue glass towers, and perpetual twilight; show the crowded eastern market, wide establishing view, no modern objects","out":"world-east-market.png"}
{"operation":"generate","prompt":"The same floating city of brass bridges, blue glass towers, and perpetual twilight; show the quiet western observatory, wide establishing view, no modern objects","out":"world-west-observatory.png"}
```

A combination is allowed only when each combined dimension is authorized. State all changes in each self-contained prompt:

```jsonl
{"operation":"generate","prompt":"A woman in a yellow raincoat walking through a misty pine forest, three-quarter portrait, muted palette, no text; change the pose to looking over her shoulder and render in gouache","out":"raincoat-01.png"}
{"operation":"generate","prompt":"A woman in a yellow raincoat walking through a misty pine forest, three-quarter portrait, muted palette, no text; change the pose to holding an open umbrella and render in gouache","out":"raincoat-02.png"}
```

## Prompt fidelity and consistency

Treat the user's prompt as authoritative. Pass it unchanged unless the user explicitly asks to rewrite, optimize, expand, translate, or otherwise modify it. Do not add, remove, reinterpret, improve, or silently correct creative details. API controls such as model, size, quality, and output path belong in CLI arguments or job fields, not in the visual prompt unless the user wants those words rendered.

If the user requires a dimension to stay consistent but has not supplied enough detail, create a concise **Consistency Anchor** for that dimension. Include the anchor verbatim in every task prompt. User-provided details always take priority over an anchor. A reference image is a stronger visual anchor than prose for preserving appearance or identity; do not invent roles or semantics for it.

Only use the user's explicit attachments or file paths. A word such as “poster”, “cover”, or “screenshot” does not imply a reference image. If no image is provided, do not ask for one and do not search for one.

## Choose the operation

- Use `generate` for one prompt and one new image.
- Use `batch` for every multi-output request, including a set of exact-prompt copies, generated variants, or multiple reference-image edits.
- Use `edit` for one output that changes or combines existing images. A batch task for the same kind of request must use `"operation":"edit"`.
- Each `generate` or `edit` task requests and saves exactly one image. Never pass a quantity option.

### Reference images

For a normal reference-image task, use the explicit `edit` operation. Repeat `--image` or list `image` paths in exactly the order supplied by the user. The CLI uploads them in that order. Use `--mask` or `mask` only when the user provides one.

If the user has not assigned roles to multiple images, do not ask, guess, assign, supplement, inspect, or reorder roles. Pass the images in user order and pass the user's prompt unchanged. Do not inspect input images on the normal path.

Inspect an input image only when the user explicitly asks for image analysis, asks the Skill to write or refine a prompt from the image, or an execution failure requires troubleshooting. After successful generation, do not inspect, score, select, compare for similarity, or visually retry the outputs.

A multi-reference batch keeps each job explicit and self-contained:

```jsonl
{"operation":"edit","prompt":"Combine the supplied images into one editorial collage. Preserve the user's composition and do not add text.","image":["inputs/subject.png","inputs/texture.png","inputs/layout.png"],"out":"collage-01.png"}
{"operation":"edit","prompt":"Combine the supplied images into one editorial collage. Preserve the user's composition and do not add text.","image":["inputs/subject.png","inputs/texture.png","inputs/layout.png"],"out":"collage-02.png"}
```

## Batch protocol

For ordinary batches, write JSONL directly and invoke the launcher once:

```bash
"<img-gen>" batch \
  --input "tmp/imagegen/jobs.jsonl" \
  --out-dir "output/imagegen"
```

Each non-empty line is one job. Every job must explicitly set `operation` to `generate` or `edit`. Supported fields are `operation`, `prompt`, `out`, `image`, `mask`, `size`, `quality`, and `model`.

- `generate` jobs must not include `image` or `mask` at all. `edit` jobs require an `image` array with at least one path and may include one `mask`.
- Every job requests exactly one image. Do not include `n` or any single-request multi-image quantity.
- Relative `image` and `mask` paths resolve from the batch JSONL directory. Relative `out` paths resolve under `--out-dir`; absolute paths remain absolute. Add an extension when needed and use unique output paths.
- The CLI preflights every operation, prompt, input file, output conflict, and duplicate resolved output before any network request. Do not compensate for preflight with workspace searches or manual checks.
- Concurrency priority is `--concurrency`, then `IMAGE_API_BATCH_CONCURRENCY`, then the CLI default of `5`. The worker pool replaces completed jobs immediately.
- By default, a failed job does not stop other jobs. Successful files remain, the summary stays in input order, and the process exits nonzero if any job fails. `--fail-fast` stops scheduling jobs not yet started while already-started jobs finish.
- The JSON summary identifies each job by `index` and `operation`, and reports successful `outputs` or an `error` reason.

Read [references/batch-format.md](references/batch-format.md) only when resolving a batch-format question or error.

## Generate one image

```bash
"<img-gen>" generate \
  --prompt "A small blue nebula in a glass bottle, studio product photo" \
  --size 1024x1024 \
  --quality auto \
  --out "output/imagegen/nebula.png"
```

Use `--prompt-file` for a prompt that is inconvenient to pass safely as one argument.

## Edit one image

State each input's role only when the user states it, and repeat invariants that the user supplied or explicitly authorized you to formulate. Preserve originals and write edits to a new path unless the user explicitly authorizes overwriting.

```bash
"<img-gen>" edit \
  --image "input/product.png" \
  --prompt "Replace only the background with a warm studio backdrop. Keep the product, label, proportions, and edges unchanged." \
  --quality auto \
  --out "output/imagegen/product-edited.png"
```

Repeat `--image` for multiple inputs. Use `--mask` when a compatible PNG mask is available.

## Sizes

If the user does not specify a size, use `--size auto`; never choose 1K, 2K, or 4K on the user's behalf. Translate explicit natural-language resolution requests to dimensions:

| Request | Square | Landscape | Portrait |
| --- | --- | --- | --- |
| 1K | `1024x1024` | `1024x576` | `576x1024` |
| 2K | `2048x2048` | `2048x1152` | `1152x2048` |
| 4K | `4096x4096` | `3840x2160` | `2160x3840` |

Use the requested orientation or infer it from the asset. Pass the requested dimensions without imposing provider-specific limits.

## Configuration and failures

- Execute the requested command first. Let the CLI report missing configuration or invalid arguments instead of probing in advance.
- Never print or request API keys in chat.
- Use `--dry-run` only when validation is requested or needed to diagnose an argument problem.
- Do not overwrite files unless the user authorizes it and `--force` is passed.
- Native transparent output is not guaranteed.
- On a failed batch, report the CLI summary and error reasons, retain and deliver successful outputs, and do not inspect or visually retry them unless the user asks for troubleshooting.

Read [references/troubleshooting.md](references/troubleshooting.md) only after a configuration, permission, retry, timeout, or provider error.

## Finish

After the launcher successfully returns generated files, deliver each successful output immediately using Markdown image syntax with its absolute output path, preserving batch input order where the summary provides it. Do not inspect or evaluate the output for subject, composition, text accuracy, requested constraints, artifacts, pixel-level difference, or similarity.
