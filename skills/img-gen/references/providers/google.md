# Google Provider

This reference records only Google-specific configuration, Model, Base URL, protocol, and error facts. Read it only after shared failure guidance leaves one of those facts unresolved; do not use it for common failure rules.

## Provider Configuration

The Google Provider block uses these user-level JSON fields:

- `baseURL`: the configured Google API base URL. The CLI appends `/v1beta` when the value does not already end in `/v1beta`.
- `apiKey`: sent as the `key` query parameter on each Google API request.
- `model`: the Google image Model identifier inserted into `/v1beta/models/<model>:predict`.

These fields belong to `providers.google`; they do not provide configuration for another Provider and do not select a Provider by themselves.

## Image API behavior

- `generate` sends a JSON `POST` request to `/v1beta/models/<model>:predict` with an `instances` array containing the prompt and `parameters.sampleCount: 1`.
- `edit` sends the same JSON prediction request with input image bytes encoded in the instance as `bytesBase64Encoded`; an optional mask is encoded in the mask image object.
- A successful response contains a `predictions` array. Each image prediction contains `bytesBase64Encoded`; the CLI decodes the bytes and saves the image through the common output path.
- Google HTTP errors are returned to the common control plane as the Provider request failure.
