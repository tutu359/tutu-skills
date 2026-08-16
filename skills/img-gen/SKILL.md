---
name: img-gen
description: Generate and edit raster images through a configurable OpenAI-compatible Image API. Use when the user asks for one or many images, illustrations, product shots, covers, website assets, visual variants, background replacements, object changes, compositing, or other image edits through a separately configured API provider.
---

# img-gen

Generate and edit images with the bundled native macOS and Windows CLI. No Python, Node.js, Go, or package installation is required during normal use.

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

For a normal generation request, call the binary immediately. Do not delay the first request with workspace searches, attachment searches, architecture checks, configuration probes, dry runs, compilation, or reference-file reads. If the user did not provide an attachment or file path, do not ask for one and do not search the workspace, clipboard, temporary folders, or other locations for one. Generate from the text prompt alone. Troubleshoot only after the binary reports an error.

In the examples below, replace `<img-gen>` with the launcher for the current operating system.

## Choose the operation

- Use `generate` for one prompt and one image. Use `batch` for distinct prompts or assets. `generate-batch` is no longer a supported command.
- Use `edit` to change or combine existing images. Inspect each input first and preserve the originals.
- Each `generate` or `edit` task requests and saves exactly one image; do not pass a quantity option.

## Prompts

Treat the user's prompt as authoritative. Pass it unchanged unless the user explicitly asks to rewrite, optimize, expand, translate, or otherwise modify it. Do not add, remove, reinterpret, or "improve" creative details on the user's behalf.

Preserve every provided detail, including intended use, subject, scene, style, composition, lighting, palette, camera treatment, exact text, constraints, and avoid items. Do not invent any of these details when they are absent. Quote text that must appear verbatim.

Keep API controls such as model, size, quality, and output path in CLI arguments rather than the visual prompt unless the user wants those words rendered in the image.

Do not infer a reference image from wording such as "poster," "cover," or "screenshot." Use an image input only when the user actually provides an attachment or an explicit file path. If no image is provided, do not ask for one and do not search for one; handle a generation request from the text prompt alone. Never pretend that a missing image exists.

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
{"operation":"generate","prompt":"A blue ceramic mug on white","out":"mug.png"}
{"operation":"generate","prompt":"A red paper kite in a clear sky","size":"1536x1024","quality":"low","out":"kite.png"}
```

Then run:

```bash
"<img-gen>" batch \
  --input "tmp/imagegen/jobs.jsonl" \
  --out-dir "output/imagegen"
```

Supported job fields are `operation`, `prompt`, `out`, `size`, `quality`, and `model`. Every job must set `operation` to `generate`, and each job requests one image. Relative `out` paths are resolved under `--out-dir`; absolute paths are unchanged. All jobs are preflighted before any network request, including output conflicts and duplicate resolved paths. Use unique output names. Read [references/batch-format.md](references/batch-format.md) only when resolving an unfamiliar batch-format question or error.

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

Read [references/troubleshooting.md](references/troubleshooting.md) only after a configuration, permission, retry, timeout, or provider error.

## Finish

After the binary successfully returns the generated image, deliver it to the user immediately using Markdown image syntax with the absolute output path: `![Generated image](/absolute/path/to/image.png)`. Do not inspect or evaluate the output for subject, composition, text accuracy, requested constraints, or artifacts.
