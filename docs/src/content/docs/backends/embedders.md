---
title: Embedders
description: Choose and configure the embedding provider grepai uses to turn code into vectors (Ollama, LM Studio, OpenAI, Synthetic, OpenRouter, Requesty).
---

An embedder turns each code chunk (and each search query) into a vector. This page helps you pick a provider and shows the configuration for each one.

## Available Embedders

| Provider | Runs | Default model | Default dimensions | API key |
|----------|------|---------------|--------------------|---------|
| `ollama` (default) | Local | `nomic-embed-text` | 768 | No |
| `lmstudio` | Local | `text-embedding-nomic-embed-text-v1.5` | 768 | No |
| `openai` | Cloud | `text-embedding-3-small` | 1536 | `OPENAI_API_KEY` |
| `synthetic` | Cloud | `hf:nomic-ai/nomic-embed-text-v1.5` | 768 | `SYNTHETIC_API_KEY` |
| `openrouter` | Cloud | `openai/text-embedding-3-small` | 1536 | `OPENROUTER_API_KEY` |
| `requesty` | Cloud | `openai/text-embedding-3-small` | 1536 | `REQUESTY_API_KEY` |

How to choose:

- **Keep code on your machine:** use `ollama` or `lmstudio`.
- **Large repository on a cloud API:** consider `openai`. It is the only provider that sends batches in parallel and retries on rate limits.
- **One key for many models:** use `openrouter` or `requesty`.

Pick the provider during `grepai init`, or pass it directly:

```bash
grepai init --provider openai
```

All providers share the same `embedder` keys in `.grepai/config.yaml`:

| Key | Used by | Description |
|-----|---------|-------------|
| `provider` | all | `ollama`, `lmstudio`, `openai`, `synthetic`, `openrouter` or `requesty` |
| `model` | all | Model name exactly as the provider expects it |
| `endpoint` | all | Base URL of the API (each provider has a default) |
| `api_key` | cloud providers | API key. If empty, grepai reads the provider's environment variable, but only when `endpoint` is the provider's official endpoint. A custom endpoint requires `api_key` |
| `dimensions` | all | Vector size. See [Changing Embedding Models](#changing-embedding-models) |
| `request_timeout_seconds` | all except `requesty` | HTTP timeout per request. Default: 60 (90 for `synthetic`) |
| `parallelism` | `openai` | Concurrent requests. Default: 4 |
| `max_retries` | `openai` | Retries on HTTP 429 and 5xx. Default: 5 |
| `max_batch_size` | `openai` | Maximum inputs per request. Default: 2000 |
| `max_batch_tokens` | `openai` | Maximum estimated tokens per request. Default: 280000 |

> **Note:** grepai does not expand environment variables in `config.yaml`. A value like `api_key: ${OPENAI_API_KEY}` is sent as that literal text. To use an environment variable, leave `api_key` out and export the variable.

> **Custom endpoints need `api_key`:** grepai sends a key read from the environment (`OPENAI_API_KEY`, `OPENROUTER_API_KEY`, `SYNTHETIC_API_KEY`, `REQUESTY_API_KEY`) only to the provider's official host over HTTPS: `api.openai.com`, `openrouter.ai`, `api.synthetic.new` or `router.requesty.ai`. A `.grepai/config.yaml` can come with a cloned repository, so its `endpoint` is not trusted with your environment keys. For any other endpoint (Azure, LiteLLM, vLLM, a proxy…), set `api_key` in the config. If you leave it empty while an environment key is set, grepai stops with an error that names the endpoint instead of sending the key. With the `openai` provider, a local server that needs no key (vLLM, TEI…) still needs a placeholder, for example `api_key: unused`: grepai refuses to start without a key.

## Ollama (Local)

### Setup

1. Install Ollama:

   ```bash
   # macOS
   brew install ollama

   # Linux
   curl -fsSL https://ollama.com/install.sh | sh
   ```

2. Start the server:

   ```bash
   ollama serve
   ```

3. Pull an embedding model:

   ```bash
   ollama pull nomic-embed-text
   ```

### Configuration

`grepai init` asks for the Ollama endpoint (default `http://localhost:11434`), so you can point it at a remote server, a Docker container or a custom port. It uses `nomic-embed-text` as the model. Edit `model` and `dimensions` to use another one.

```yaml
embedder:
  provider: ollama
  model: nomic-embed-text
  endpoint: http://localhost:11434
  dimensions: 768
```

grepai sends up to 64 chunks per request to Ollama's `/api/embed` endpoint. If the server does not have that endpoint (older Ollama versions), grepai switches to the legacy `/api/embeddings` endpoint and sends one chunk per request.

If a chunk is longer than the model's context window, grepai splits that chunk into smaller pieces and tries again.

### Available Models

Any Ollama embedding model works. Common choices:

| Model | Dimensions | Languages |
|-------|------------|-----------|
| `nomic-embed-text` | 768 | English |
| `nomic-embed-text-v2-moe` | 768 | Multilingual |
| `bge-m3` | 1024 | Multilingual |
| `mxbai-embed-large` | 1024 | English |
| `all-minilm` | 384 | English |

Set `dimensions` to match the model you choose.

### Multilingual Support

`nomic-embed-text` is trained mostly on English. For comments and identifiers in other languages (Korean, Chinese, French, and so on), use a multilingual model such as `nomic-embed-text-v2-moe`. It has the same dimensions (768) as the default, so you only change `model`:

```bash
ollama pull nomic-embed-text-v2-moe
```

```yaml
embedder:
  provider: ollama
  model: nomic-embed-text-v2-moe
  dimensions: 768
```

You still need to [re-index](#re-indexing-after-model-change) after switching.

### Troubleshooting

```bash
# Check that Ollama is running
curl http://localhost:11434/api/tags

# Test an embedding
curl http://localhost:11434/api/embed -d '{
  "model": "nomic-embed-text",
  "input": "Hello world",
  "truncate": false
}'
```

If large batches time out on slow hardware, raise `request_timeout_seconds`:

```yaml
embedder:
  provider: ollama
  request_timeout_seconds: 300
```

## LM Studio (Local)

LM Studio runs embedding models locally behind an OpenAI-compatible API.

### Setup

1. Download and install [LM Studio](https://lmstudio.ai/).
2. Load an embedding model (for example, a Nomic embed model).
3. Start the local server (default: `http://127.0.0.1:1234`).

### Configuration

`grepai init` asks for the LM Studio endpoint (default `http://127.0.0.1:1234`).

```yaml
embedder:
  provider: lmstudio
  model: text-embedding-nomic-embed-text-v1.5
  endpoint: http://127.0.0.1:1234
  dimensions: 768
```

grepai calls `/v1/embeddings` and sends all chunks of a file in one request. Chunks that exceed the model's context window are split and retried.

### Available Models

Any embedding model loaded in LM Studio works. Use the model identifier that LM Studio reports, and set `dimensions` to match it:

```bash
curl http://127.0.0.1:1234/v1/models
```

### Troubleshooting

```bash
# Check that the LM Studio server is running
curl http://127.0.0.1:1234/v1/models

# Test an embedding
curl http://127.0.0.1:1234/v1/embeddings \
  -H "Content-Type: application/json" \
  -d '{
    "model": "text-embedding-nomic-embed-text-v1.5",
    "input": ["Hello world"]
  }'
```

## OpenAI (Cloud)

### Setup

1. Get an API key from the [OpenAI Platform](https://platform.openai.com/api-keys).
2. Export it:

   ```bash
   export OPENAI_API_KEY=sk-...
   ```

### Configuration

```yaml
embedder:
  provider: openai
  model: text-embedding-3-small
  endpoint: https://api.openai.com/v1
  parallelism: 4
```

`grepai init` leaves `dimensions` unset, so the model returns its native size. If you set `dimensions`, grepai sends it to the API as the `dimensions` parameter, which asks the model for shorter vectors.

### Azure OpenAI / Microsoft Foundry

Any OpenAI-compatible API works with `provider: openai` and a custom `endpoint`. grepai sends `POST {endpoint}/embeddings` with an `Authorization: Bearer` header. With a custom endpoint, the key must be in `api_key`: `OPENAI_API_KEY` is only sent to `api.openai.com`.

```yaml
embedder:
  provider: openai
  model: text-embedding-3-small   # on Azure, use your deployment name
  endpoint: https://YOUR-RESOURCE.openai.azure.com/openai/v1
  api_key: your-azure-key
```

Leave `dimensions` unset for models that do not accept the parameter, such as `text-embedding-ada-002`.

### Available Models

| Model | Dimensions |
|-------|------------|
| `text-embedding-3-small` (default) | 1536 |
| `text-embedding-3-large` | 3072 |

```bash
grepai init --provider openai --model text-embedding-3-large
```

For current prices, see [OpenAI pricing](https://openai.com/api/pricing/).

### Parallelism & Rate Limiting

During indexing, grepai groups chunks from many files into batches (up to 2,000 inputs and about 280,000 estimated tokens each) and sends up to `parallelism` batches at the same time.

```yaml
embedder:
  provider: openai
  parallelism: 4     # concurrent requests (default: 4)
  max_retries: 5     # retries per batch on 429 or 5xx (default: 5)
```

OpenAI-compatible endpoints (self-hosted vLLM, TEI, proxies…) often accept much smaller requests and time out or fail on the defaults. Lower the batch limits for them; they also apply to the watcher, which embeds one file per request:

```yaml
embedder:
  provider: openai
  endpoint: https://embeddings.example.com/v1
  max_batch_size: 100       # inputs per request (default: 2000)
  max_batch_tokens: 50000   # estimated tokens per request (default: 280000)
```

Tokens are estimated at about 4 characters per token. A single chunk larger than `max_batch_tokens` is still sent alone.

How errors are handled:

- **429 and 5xx:** the batch is retried with exponential backoff (1s, doubling, capped at 32s, plus random jitter). If the response has a `Retry-After` header, grepai waits that long instead (at most 60s).
- **Repeated 429s:** after 3 consecutive 429 responses, grepai halves the number of workers (minimum 1). After 10 consecutive successes, it adds one worker back, up to `parallelism`.
- **Batch too large:** if OpenAI rejects a batch for exceeding the maximum request size, grepai splits it in half and retries.
- **Other 4xx errors:** not retried.

#### Recommended Parallelism by Tier

Your rate limits depend on your OpenAI account tier. Start with the default (4). If indexing runs without 429 errors, raise it; if you see many retries, lower it.

To see your limits, inspect the rate limit headers of a test request:

```bash
curl -s -D - https://api.openai.com/v1/embeddings \
  -H "Authorization: Bearer $OPENAI_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"model": "text-embedding-3-small", "input": ["test"]}' \
  -o /dev/null 2>&1 | grep -i x-ratelimit
```

`x-ratelimit-limit-requests` is your requests-per-minute cap and `x-ratelimit-limit-tokens` is your tokens-per-minute cap.

### Cost Estimation

You pay for the tokens you embed. As a rough guide, grepai estimates about 4 characters per token, so a 1 MB codebase is about 250,000 tokens for the first full index. After that, only changed files are embedded again, and chunks whose content is already in the index reuse their stored vector.

## Synthetic (Cloud)

[Synthetic](https://synthetic.new) serves open embedding models through an OpenAI-compatible API.

```bash
export SYNTHETIC_API_KEY=your-key
```

```yaml
embedder:
  provider: synthetic
  model: hf:nomic-ai/nomic-embed-text-v1.5
  endpoint: https://api.synthetic.new/openai/v1
  dimensions: 768
```

- If `api_key` and `SYNTHETIC_API_KEY` are both empty, grepai falls back to `OPENAI_API_KEY`.
- grepai always sends `dimensions` in the request (768 by default).
- The default request timeout is 90 seconds.

## OpenRouter (Cloud)

[OpenRouter](https://openrouter.ai) is a gateway to embedding models from several vendors.

```bash
export OPENROUTER_API_KEY=your-key
```

```yaml
embedder:
  provider: openrouter
  model: openai/text-embedding-3-small
  endpoint: https://openrouter.ai/api/v1
```

`grepai init` offers three models. grepai infers the dimensions for them when `dimensions` is unset:

| Model | Dimensions | `--model` shortcut |
|-------|------------|--------------------|
| `openai/text-embedding-3-small` (default) | 1536 | `text-embedding-3-small` |
| `openai/text-embedding-3-large` | 3072 | `text-embedding-3-large` |
| `qwen/qwen3-embedding-8b` | 4096 | `qwen3-embedding-8b` |

```bash
grepai init --provider openrouter --model qwen3-embedding-8b
```

For any other model, set `dimensions` yourself. If `api_key` and `OPENROUTER_API_KEY` are both empty, grepai falls back to `OPENAI_API_KEY`.

## Requesty (Cloud)

[Requesty](https://requesty.ai) is another multi-vendor gateway.

```bash
export REQUESTY_API_KEY=your-key
```

```yaml
embedder:
  provider: requesty
  model: openai/text-embedding-3-small
  endpoint: https://router.requesty.ai/v1
```

- grepai infers 3072 dimensions for `openai/text-embedding-3-large` and 1536 for other models. Set `dimensions` for models with another size.
- If `api_key` and `REQUESTY_API_KEY` are both empty, grepai falls back to `OPENAI_API_KEY`.
- The request timeout is fixed at 60 seconds (`request_timeout_seconds` is ignored).

## Changing Embedding Models

You can use any embedding model your provider offers. Two keys matter:

| Key | Description |
|-----|-------------|
| `model` | The exact model name the provider expects |
| `dimensions` | The size of the vectors the model produces |

When `dimensions` is unset, grepai assumes 768 for `ollama`, `lmstudio` and `synthetic`, and 1536 for `openai`, `openrouter` and `requesty` (3072 or 4096 for the models listed above). PostgreSQL and Qdrant use this value to size the vector column or collection, so it must match the model.

### Finding Model Dimensions

**Ollama:**

```bash
curl -s http://localhost:11434/api/embed \
  -d '{"model": "MODEL_NAME", "input": "test"}' | jq '.embeddings[0] | length'
```

**LM Studio:**

```bash
curl -s http://127.0.0.1:1234/v1/embeddings \
  -H "Content-Type: application/json" \
  -d '{"model": "MODEL_NAME", "input": ["test"]}' | jq '.data[0].embedding | length'
```

### Re-indexing After Model Change

Vectors from different models are not compatible. grepai records which embedder built each index (provider, model and dimensions, in `.grepai/index-meta.json`; the endpoint is not part of it), so changing one of them is detected:

- `grepai search` (CLI and MCP) refuses to run on the old index and tells you to rebuild it.
- `grepai index` and `grepai watch` remove the project's old vectors and re-embed every file.

So after changing `provider`, `model` or `dimensions`, run:

```bash
grepai watch --stop          # if a background watcher is running
grepai index                 # or: grepai watch
```

An index built by a version older than this check has no metadata file: it is adopted as is the first time it is indexed again.

## Requesting a New Embedder

grepai works [issue first](/grepai/contributing/) and does not accept pull requests. To ask for another provider, open a [feature request](https://github.com/yoanbernabeu/grepai/issues/new?template=feature_request.yml) with the provider, the model you want to use and your use case.

Many services expose an OpenAI-compatible API: they usually work today with the `openai` provider and a custom `endpoint`.
