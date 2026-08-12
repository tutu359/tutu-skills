---
name: img-gen
description: Generate and edit raster images through a configurable OpenAI-compatible Image API. Use when the user asks for one or many images, illustrations, product shots, covers, website assets, visual variants, background replacements, object changes, compositing, or other image edits through a separately configured API provider.
---

# img-gen

Generate and edit images with the bundled native macOS and Windows CLI. No Python, Node.js, Go, or package installation is required during normal use.

## Fast path

For every normal `generate`, `edit`, or `generate-batch` request:

1. Prepare only the prompt and files required by the request.
2. Select the launcher from the current operating system, then run it directly:
   - macOS: `bin/img-gen`
   - Windows: `bin\img-gen.cmd`
   Both launchers select the correct CPU architecture.
3. Let the CLI validate API configuration, output paths, and arguments. Do not pre-check environment variables.
4. On macOS, do not run `chmod` preemptively. Repair execution permission only after a permission-denied error, then retry once.
5. In the same session, reuse established output conventions and request settings unless the user changes them.

Do not delay the first request with architecture checks, configuration probes, dry runs, compilation, or reference-file reads that the request does not require.

In the examples below, replace `<img-gen>` with the launcher for the current operating system.

## Choose the operation

- Use `generate` for one prompt. Use `--n` only for variants of that same prompt.
- Use `edit` to change or combine existing images. Inspect each input first and preserve the originals.
- Use `generate-batch` for distinct prompts or assets.

## Prompts

Preserve detailed prompts. For a simple request, add only details needed to make the image usable; do not force every prompt into a long template. Do not invent brands, people, slogans, or unrelated objects.

Include relevant items such as intended use, subject, scene, style, composition, lighting, palette, exact text, constraints, and avoid items. Quote text that must appear verbatim.

Keep API controls such as model, size, quality, and output path in CLI arguments rather than the visual prompt unless the user wants those words rendered in the image.

## Generate one image

```bash
"<img-gen>" generate \
  --prompt "A small blue nebula in a glass bottle, studio product photo" \
  --size 1024x1024 \
  --quality auto \
  --out "output/imagegen/nebula.png"
```

Use `--prompt-file` for a prompt that is inconvenient to pass safely as one argument.

## Edit an image

State each input's role and repeat invariants so unrelated details do not drift.

```bash
"<img-gen>" edit \
  --image "input/product.png" \
  --prompt "Replace only the background with a warm studio backdrop. Keep the product, label, proportions, and edges unchanged." \
  --quality auto \
  --out "output/imagegen/product-edited.png"
```

Repeat `--image` for multiple inputs. Use `--mask` when a compatible PNG mask is available. Always write edits to a new path unless the user explicitly authorizes overwriting.

## Generate a batch

For ordinary batches, write JSONL directly without first reading another file. Each non-empty line is one job:

```jsonl
{"prompt":"A blue ceramic mug on white","out":"mug.png"}
{"prompt":"A red paper kite in a clear sky","size":"1536x1024","quality":"low","n":2,"out":"kite.png"}
```

Then run:

```bash
"<img-gen>" generate-batch \
  --input "tmp/imagegen/jobs.jsonl" \
  --out-dir "output/imagegen"
```

Supported job fields are `prompt`, `out`, `size`, `quality`, `n`, and `model`. Use unique output names. Read [references/batch-format.md](references/batch-format.md) only when resolving an unfamiliar batch-format question or error.

## Sizes

If the user does not specify a size, use `--size auto`. Translate natural-language resolution requests to dimensions:

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

Read [references/troubleshooting.md](references/troubleshooting.md) only after a configuration, permission, retry, timeout, or provider error.

## Finish

Inspect generated outputs for the subject, composition, text accuracy, requested constraints, and artifacts. Report absolute output paths, prompt or prompt set, size, quality, and model.
