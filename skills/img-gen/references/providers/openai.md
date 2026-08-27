# OpenAI Provider

This reference records only OpenAI-specific configuration, Model, Base URL, protocol, and error facts. Read it only after shared failure guidance leaves one of those facts unresolved; do not use it for common failure rules.

## Provider Configuration

The OpenAI Provider block uses these user-level JSON fields:

- `baseURL`: the configured OpenAI API base URL. The CLI appends `/v1` when the value does not already end in `/v1`.
- `apiKey`: sent as an HTTP Bearer token in the `Authorization` header.
- `model`: the OpenAI image Model identifier sent in each request.

These fields belong to `providers.openai`; they do not provide configuration for another Provider and do not select a Provider by themselves.

## Image API behavior

- `generate` sends a JSON `POST` request to `/v1/images/generations` with `model`, `prompt`, `size`, `quality`, and `n: 1`.
- `edit` sends a multipart `POST` request to `/v1/images/edits` with the same text fields and one or more `image` parts. An optional `mask` part follows the image parts.
- A successful response contains a `data` array. Each image entry may contain `b64_json` or a URL; the CLI decodes or downloads the returned image and saves it through the common output path.
- The OpenAI Provider does not infer a Model or change the requested controls based on an endpoint's advertised capabilities.
- OpenAI HTTP errors are returned to the common control plane as the Provider request failure.
