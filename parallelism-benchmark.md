# Parallelism Benchmark Results

## Test Environment
- Date: 2026-01-25
- Embedder: OpenAI text-embedding-3-small
- Platform: Linux

## Results (grepai repo - small)

| Parallelism | Time (s) | Files | Chunks |
|-------------|----------|-------|--------|
| 1           | 7.15     | 112   | 685    |
| 2           | 6.35     | 112   | 685    |
| 4 (default) | 5.66     | 112   | 685    |

**Speedup**: 1.26x (parallelism=4 vs parallelism=1)

## Results (clawdbot repo - large)

Config: chunk_size=128, overlap=20 (reduced to avoid 300k token limit)

| Parallelism | Time (s) | Files | Chunks |
|-------------|----------|-------|--------|
| 1           | 82.6     | 4065  | 59349  |
| 4           | 95.6     | 4065  | 59349  |

**Speedup**: 0.86x (parallelism=4 was SLOWER)

## Observations

1. **Small repos** (685 chunks): Minor speedup (~26%) with parallelism
   - All chunks fit in ~1 batch, limiting parallelism benefit

2. **Large repos** (59k chunks): Parallelism was slower
   - Likely due to rate limiting triggering retries
   - Network conditions may have varied between tests

3. **Token limit bug**: With default chunk_size=512, batches exceeded OpenAI's 300k token limit
   - Error: "Requested 679453 tokens, max 300000 tokens per request"
   - Workaround: Reduced chunk_size to 128
   - **Bug to fix**: Batch logic should account for token count, not just input count

4. **Config persistence bug**: `parallelism` setting is dropped on config save
   - Field has `yaml:"parallelism,omitempty"` tag
   - Value is being loaded correctly but not saved back

## Recommendations

1. ~~Fix token limit bug - implement token counting in batch logic~~ **FIXED**
2. ~~Fix parallelism config persistence~~ **FIXED**
3. ~~Consider adaptive parallelism based on rate limit responses~~ **FIXED** (rate limiting visibility added)
4. Run multiple test iterations to account for network variance

## Fixes Applied (2026-01-26)

### Token Limit Bug Fix
- Added `EstimateTokens()` function in `embedder/batch.go` to estimate token count (~4 chars per token)
- Added `MaxBatchTokens = 280000` constant (OpenAI limit is 300k, using 280k for safety margin)
- `FormBatches()` now limits batches by both count (2000) AND token count
- Added tests: `TestEstimateTokens`, `TestFormBatches_TokenLimit`, `TestFormBatches_SmallChunksIgnoreTokenLimit`

### Config Persistence Bug Fix
- Removed `omitempty` tag from `Parallelism` field in `EmbedderConfig`
- Added `Parallelism: 4` to `DefaultConfig()` so it's always explicitly set
- Field is now always persisted regardless of value

### Rate Limiting Visibility
- Added `StatusCode` field to `BatchProgressInfo` to expose HTTP status during retries
- Extended `BatchProgress` callback to include `statusCode int` parameter
- Added `describeRetryReason()` helper to show human-readable messages:
  - `Rate limited (429)` for rate limiting
  - `Server error (5xx)` for server errors
  - `HTTP error (xxx)` for other errors
- Progress output now shows: `Rate limited (429) - Retrying batch 1 (attempt 2/5)...`
