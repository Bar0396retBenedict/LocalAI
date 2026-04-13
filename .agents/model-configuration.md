# Model Configuration Guide

This guide covers how to configure models in LocalAI, including YAML configuration files, model parameters, and backend-specific settings.

## Overview

LocalAI uses YAML configuration files to define model behavior, backend selection, and inference parameters. Each model can have its own configuration file stored in the models directory.

## Configuration File Structure

Model configuration files are placed in the `models/` directory (or the directory specified by `--models-path`) and follow this naming convention:

```
models/
  my-model.yaml
  my-model.bin          # or .gguf, .ggml, etc.
```

### Basic Configuration

```yaml
# models/my-model.yaml
name: my-model
backend: llama-cpp
parameters:
  model: my-model.bin
  context_size: 4096
  threads: 4
  f16: true
```

### Full Configuration Reference

```yaml
# Model identity
name: my-model                    # Name used in API requests
backend: llama-cpp                # Backend to use (llama-cpp, whisper, diffusers, etc.)
description: "My custom model"    # Optional description

# File parameters
parameters:
  model: my-model.gguf            # Model file (relative to models dir or absolute path)
  context_size: 2048              # Context window size
  seed: -1                        # Random seed (-1 for random)
  threads: 4                      # CPU threads
  f16: true                       # Use float16 precision
  gpu_layers: 0                   # Layers to offload to GPU (0 = CPU only)
  mmap: true                      # Memory-map the model file
  mlock: false                    # Lock model in RAM

# Template configuration
template:
  chat: "{{.Input}}"              # Chat completion template
  completion: "{{.Input}}"        # Text completion template
  chat_message: "{{.RoleName}}: {{.Content}}\n"  # Per-message template

# Default inference parameters
parameters:
  top_k: 40
  top_p: 0.95
  temperature: 0.9
  repeat_penalty: 1.1
  max_tokens: 512

# Feature flags
stop_words:                       # Strings that stop generation
  - "User:"
  - "###"
roles:                            # Role name mappings
  user: "User"
  system: "System"
  assistant: "Assistant"
```

## Backend-Specific Configuration

### llama-cpp

```yaml
backend: llama-cpp
parameters:
  model: llama-2-7b.Q4_K_M.gguf
  context_size: 4096
  gpu_layers: 35          # Set >0 to use GPU acceleration
  f16: true
  mmap: true
  numa: false             # NUMA optimization
  low_vram: false         # Reduce VRAM usage
  embeddings: false       # Enable embeddings endpoint
```

### whisper (Speech-to-Text)

```yaml
backend: whisper
parameters:
  model: whisper-base.en.bin
  language: en
  translate: false
```

### diffusers (Image Generation)

```yaml
backend: diffusers
parameters:
  model: stabilityai/stable-diffusion-2-1
  img2img: false
  pipeline_type: StableDiffusionPipeline
  cuda: true
  scheduler_type: euler_a
```

## Environment Variable Overrides

Configuration values can be overridden via environment variables using the pattern `LOCALAI_MODEL_<PARAM>`:

```bash
export LOCALAI_THREADS=8
export LOCALAI_CONTEXT_SIZE=8192
export LOCALAI_GPU_LAYERS=40
```

## Template Syntax

LocalAI uses Go's `text/template` syntax for prompt templates.

### Available Variables

| Variable | Description |
|---|---|
| `.Input` | The full input text |
| `.Instruction` | System instruction |
| `.RoleName` | Current message role |
| `.Content` | Current message content |
| `.Messages` | All chat messages |

### Example: ChatML Template

```yaml
template:
  chat_message: |2
    <|im_start|>{{.RoleName}}
    {{.Content}}<|im_end|>
  chat: |2
    {{.Input}}
    <|im_start|>assistant
```

### Example: Llama-2 Chat Template

```yaml
template:
  chat_message: "[INST] {{if eq .RoleName \"system\"}}<<SYS>>\n{{.Content}}\n<</SYS>>\n\n{{else}}{{.Content}} [/INST]{{end}}"
  chat: "{{.Input}}"
```

## Validation

To validate a model configuration without loading the full model:

```bash
local-ai validate --model-path models/my-model.yaml
```

## Common Issues

### Model not found
- Ensure the model file path in `parameters.model` is correct
- Check that the file exists in `--models-path`
- Relative paths are resolved against `--models-path`

### Out of memory
- Reduce `context_size`
- Enable `mmap: true`
- Reduce `gpu_layers` or set to 0 for CPU-only
- Use a more quantized model (e.g., Q4 instead of Q8)

### Slow inference
- Increase `threads` (but don't exceed physical CPU cores)
- Enable GPU offloading with `gpu_layers`
- Enable `f16: true` for faster computation
