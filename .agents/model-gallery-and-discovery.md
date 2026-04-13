# Model Gallery and Discovery

This guide covers how LocalAI's model gallery system works, how models are discovered, fetched, and installed from remote galleries.

## Overview

LocalAI supports a gallery system that allows users to browse and install pre-configured models. Galleries are collections of model definitions hosted remotely (e.g., on GitHub) and referenced by URL.

## Gallery Configuration

Galleries are configured in the LocalAI startup configuration or via environment variables:

```yaml
# config.yaml
galleries:
  - name: localai
    url: https://raw.githubusercontent.com/mudler/LocalAI-model-gallery/main/index.yaml
  - name: community
    url: https://raw.githubusercontent.com/my-org/my-gallery/main/index.yaml
```

Or via environment variable:
```bash
GALLERIES='[{"name":"localai","url":"https://..."}]'
```

## Gallery Index Format

A gallery index is a YAML file listing available models:

```yaml
- name: mistral-7b-openorca
  urls:
    - https://raw.githubusercontent.com/mudler/LocalAI-model-gallery/main/mistral-7b-openorca.yaml
  license: apache-2.0
  description: "Mistral 7B fine-tuned on OpenOrca dataset"
  tags:
    - llm
    - mistral
    - chat
  icon: https://example.com/icon.png

- name: whisper-base
  urls:
    - https://raw.githubusercontent.com/mudler/LocalAI-model-gallery/main/whisper-base.yaml
  license: mit
  description: "OpenAI Whisper base model for speech recognition"
  tags:
    - audio
    - transcription
```

## Model Definition Format

Each model in the gallery has a YAML definition:

```yaml
# mistral-7b-openorca.yaml
name: mistral-7b-openorca
description: "Mistral 7B fine-tuned on OpenOrca"
license: apache-2.0
urls:
  - https://huggingface.co/TheBloke/Mistral-7B-OpenOrca-GGUF/resolve/main/mistral-7b-openorca.Q4_K_M.gguf

# Files to download
files:
  - filename: mistral-7b-openorca.Q4_K_M.gguf
    sha256: "abc123..."
    uri: https://huggingface.co/TheBloke/Mistral-7B-OpenOrca-GGUF/resolve/main/mistral-7b-openorca.Q4_K_M.gguf

# Model configuration (merged into the model's config file)
config_file: |
  name: mistral-7b-openorca
  backend: llama-cpp
  parameters:
    model: mistral-7b-openorca.Q4_K_M.gguf
    temperature: 0.7
    top_p: 0.9
  context_size: 4096
  f16: true
  template:
    chat: |
      {{.Input}}
    completion: |
      {{.Input}}
```

## API Endpoints

### List Available Gallery Models

```
GET /models/available
```

Returns all models from all configured galleries.

**Response:**
```json
[
  {
    "name": "mistral-7b-openorca",
    "description": "Mistral 7B fine-tuned on OpenOrca",
    "tags": ["llm", "mistral", "chat"],
    "license": "apache-2.0",
    "gallery": {"name": "localai", "url": "https://..."}
  }
]
```

### Install a Gallery Model

```
POST /models/apply
```

**Request body:**
```json
{
  "id": "localai@mistral-7b-openorca",
  "name": "my-mistral",
  "overrides": {
    "parameters": {
      "temperature": 0.5
    }
  }
}
```

The `id` format is `gallery-name@model-name`. The optional `name` field overrides the installed model name. The `overrides` field allows patching the model config at install time.

**Response:**
```json
{
  "uuid": "job-uuid-here",
  "status": "http://localhost:8080/models/jobs/job-uuid-here"
}
```

Installation is asynchronous. Poll the job status endpoint to track progress.

### Check Installation Job Status

```
GET /models/jobs/{uuid}
```

**Response:**
```json
{
  "uuid": "job-uuid-here",
  "progress": 75,
  "downloaded_size": "1.2GB",
  "file_name": "mistral-7b-openorca.Q4_K_M.gguf",
  "error": null,
  "processed": false
}
```

When `processed` is `true`, the model is installed and ready.

### Delete an Installed Model

```
POST /models/delete
```

**Request body:**
```json
{
  "id": "localai@mistral-7b-openorca"
}
```

## Installing Models via CLI

```bash
# List available models
local-ai models list

# Install a model
local-ai models install localai@mistral-7b-openorca

# Install with a custom name
local-ai models install localai@mistral-7b-openorca --name my-mistral
```

## Custom Galleries

You can host your own gallery by:

1. Creating an `index.yaml` with model entries
2. Creating individual model YAML definition files
3. Hosting them on any HTTP server (GitHub raw, S3, etc.)
4. Adding your gallery URL to LocalAI's configuration

## File Integrity Verification

LocalAI verifies downloaded files using SHA256 checksums defined in the model YAML. If a checksum is provided and the download doesn't match, the installation fails and the file is removed.

## Overrides at Install Time

The `overrides` field in the install request uses a deep-merge strategy against the base model config. This allows customizing:

- Model parameters (temperature, top_p, etc.)
- Context size
- Backend selection
- Template overrides
- Any other valid model configuration key

See `.agents/model-configuration.md` for all available configuration keys.

## Troubleshooting

- **Gallery not loading**: Check network access to the gallery URL; verify the YAML format is valid
- **Download failing**: Check disk space; verify the model URL is accessible; check SHA256 if provided
- **Model not appearing after install**: Check the job status endpoint; look at LocalAI logs for errors during config file writing
- **Slow downloads**: Large GGUF files can be several GB; consider using a local mirror or caching proxy
