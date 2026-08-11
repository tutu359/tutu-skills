---
name: img-gen
description: Generate or edit raster images through a configurable OpenAI-compatible Image API. Use when the user asks for one or many images, illustrations, product shots, covers, website assets, visual variants, background replacements, object changes, compositing, or other image edits through a separately configured API provider.
---

# img-gen

Generate and edit images with the bundled native CLI. This skill is host-neutral and can be used by Agent Skills-compatible coding agents such as Claude Code and Codex. No Python, Node.js, Go, or package installation is required during normal use.

## Select the executable

Choose once from the current Mac CPU architecture:

- Apple Silicon: `bin/img-gen-darwin-arm64`
- Intel: `bin/img-gen-darwin-amd64`

Run `chmod +x <executable>` if execute permission was not preserved. Do not compile from source during normal use.

## Workflow

1. Decide whether the request is a new image, an edit, or multiple distinct assets or variants.
2. Collect the prompt, intended use, exact text, visual constraints, and avoid items.
3. Shape the prompt only as much as needed. Preserve detailed prompts; clarify generic prompts without inventing brands, people, slogans, or unrelated objects.
4. Run `generate` for one prompt, `edit` for changes to existing images, or `generate-batch` for JSONL jobs.
5. Inspect each output for subject, composition, text accuracy, constraints, and visible artifacts.
6. If revision is needed, change one targeted aspect per iteration and re-check.
7. Report absolute output paths, the final prompt or prompt set, size, quality, and model.

## Prompt structure

Use only relevant lines:

```text
Asset type: <where the image will be used>
Primary request: <the user's request>
Scene/backdrop: <environment>
Subject: <main subject>
Style/medium: <photo, illustration, 3D, etc.>
Composition/framing: <camera angle, crop, placement, negative space>
Lighting/mood: <lighting and mood>
Color palette: <palette notes>
Text (verbatim): "<exact text>"
Constraints: <must keep or include>
Avoid: <must not include>
```

Do not add detail merely to fill the schema. For text in images, quote it verbatim and request exact rendering.

## Generate one image

```bash
"<skill-dir>/bin/img-gen-darwin-arm64" generate \
  --prompt "A small blue nebula in a glass bottle, studio product photo" \
  --size 1024x1024 \
  --quality auto \
  --out "output/imagegen/nebula.png"
```

Use `--prompt-file` for long prompts. Use `--n` only for variants of the same prompt. Distinct assets belong in separate calls or a batch.

## Edit an image

Inspect each input image before editing. State its role and repeat invariants in the prompt so unrelated details do not drift.

```bash
"<skill-dir>/bin/img-gen-darwin-arm64" edit \
  --image "input/product.png" \
  --prompt "Replace only the background with a warm studio backdrop. Keep the product, label, proportions, and edges unchanged." \
  --quality auto \
  --out "output/imagegen/product-edited.png"
```

Repeat `--image` for multiple reference or compositing inputs. Use `--mask mask.png` for a localized edit when a compatible PNG mask is available. Preserve originals and always write edits to a new output path.

## Resolution requests

Users may request image size in natural language. Translate the request into `--size WIDTHxHEIGHT`; do not pass `1K`, `2K`, or `4K` directly to the CLI.

| Request | Square | Landscape | Portrait |
| --- | --- | --- | --- |
| 1K | `1024x1024` | `1024x576` | `576x1024` |
| 2K | `2048x2048` | `2048x1152` | `1152x2048` |
| 4K | `4096x4096` | `3840x2160` | `2160x3840` |

Use the user's stated orientation or infer it from the requested asset. If orientation is not implied, default to a square image and state the chosen size. Size controls output resolution; `--quality` is a separate quality setting. The configured API provider ultimately decides which dimensions it accepts.

## Generate a batch

Read [references/batch-format.md](references/batch-format.md) before preparing a batch. Then run:

```bash
"<skill-dir>/bin/img-gen-darwin-arm64" generate-batch \
  --input "tmp/imagegen/jobs.jsonl" \
  --out-dir "output/imagegen" \
  --concurrency 2
```

## Configuration and safety

- Require an API base URL through `IMAGE_API_BASE_URL` or `--base-url`. Do not assume or silently select an API provider.
- Require `IMAGE_API_KEY` for network requests. Never place it in a command, file, prompt, log, or response.
- If the key is absent, tell the user to set it locally and confirm when ready. Never ask them to paste it into chat.
- Resolve the model in this order: `--model`, `IMAGE_API_MODEL`, then `gpt-image-2`.
- Resolve total request attempts in this order: `--max-attempts`, `IMAGE_API_MAX_ATTEMPTS`, then `3`. Three attempts means one initial request and up to two retries.
- Default to size `1024x1024` and quality `auto`.
- Use `--dry-run` to validate a configured request without network access or requiring an API key.
- Save project-bound assets inside the current project. The CLI default is `output/imagegen/`.
- Do not overwrite files unless the user explicitly authorizes it and `--force` is passed.
- Native transparent output is not guaranteed. Do not promise it or silently switch tools or models.

## Failure handling

- The CLI retries network timeouts and HTTP 429/500/502/503/504/524 failures with bounded backoff and randomized delay. It honors a valid `Retry-After` response header, capped at 30 seconds.
- Before each retry, the CLI prints the attempt number, a non-sensitive failure category, and the delay. It never prints request headers, response bodies, or keys.
- On repeated timeout, suggest `--quality low`, a square size, fewer concurrent jobs, or a later retry.
- Do not retry authentication, validation, or other ordinary 4xx errors.
- Never expose an Authorization header or full key when reporting errors.
