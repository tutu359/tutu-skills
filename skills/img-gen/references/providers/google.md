# Google Provider

This reference records only Google-specific configuration, Model, Base URL, and protocol facts. Read it only after shared failure guidance leaves one of those facts unresolved; do not use it for common failure rules.

## Provider Configuration

The Google Provider block uses these user-level JSON fields:

- `baseURL`: the configured Google API base URL. The CLI appends `/v1beta` when the value does not already end in `/v1beta`.
- `apiKey`: sent as the `key` query parameter on each Google API request.
- `model`: the Google image Model identifier inserted into `/v1beta/models/<model>:generateContent`.

These fields belong to `providers.google`; they do not provide configuration for another Provider and do not select a Provider by themselves.

## Image API behavior

- `generate` sends a JSON `POST` request to `/v1beta/models/<model>:generateContent` with `contents: [{parts: [{text: <prompt>}]}]` and `generationConfig: {responseModalities: ["IMAGE"]}`.
- `edit` sends the same `generateContent` request shape. Its first content's parts contain the prompt as a text part followed by each user-supplied reference image in order as an `inlineData` part with `mimeType` and base64 `data`.
- A successful response must contain exactly one image in `candidates[0].content.parts[]` as `inlineData.data`; the CLI decodes that base64 data and saves the image through the common output path.
- A response with zero or more than one image is invalid.
