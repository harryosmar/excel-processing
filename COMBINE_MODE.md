# Combine Mode Guide

Combine multiple Excel chunk files from S3 into one large Excel file with memory-efficient streaming.

## Overview

**Combine mode** reads multiple chunk files from S3 and merges them into a single Excel file, then uploads the combined file back to S3.

## How It Works

```
Multiple Chunks (S3) → Stream Read → Stream Write → Combined Excel → Upload to S3
```

### Process Flow

1. Lists all chunk files in S3 with matching prefix
2. Downloads each chunk file one at a time (streaming)
3. Reads rows from each chunk using streaming
4. Writes rows to combined file using stream writer
5. Skips duplicate headers (only writes header once)
6. Uploads final combined file to S3
7. Memory-efficient: only one chunk in memory at a time

## Quick Start

### Enable Combine Mode

```bash
export PROCESSING_MODE=combine
export CHUNK_PREFIX=100mb_chunk_
export OUTPUT_FILE=100mb_combined.xlsx
export S3_ENDPOINT=http://localhost:9000
export S3_FORCE_PATH_STYLE=true
export AWS_ACCESS_KEY_ID=minioadmin
export AWS_SECRET_ACCESS_KEY=minioadmin
export S3_BUCKET=excel-files

go run main.go
```

### Expected Output

```
Mode: Combine chunk files into one Excel file
Chunk prefix: 100mb_chunk_
Output file: 100mb_combined.xlsx
Starting chunk combination from S3...
Bucket: excel-files, Prefix: 100mb_chunk_
Found 58 chunk files to combine
Processing chunk 1/58: 100mb_chunk_0001.xlsx
✓ Processed 100mb_chunk_0001.xlsx: 10000 rows (total: 10000)
Processing chunk 2/58: 100mb_chunk_0002.xlsx
✓ Processed 100mb_chunk_0002.xlsx: 10000 rows (total: 20000)
...
Processing chunk 58/58: 100mb_chunk_0058.xlsx
✓ Processed 100mb_chunk_0058.xlsx: 8000 rows (total: 578000)
Writing combined file to buffer...
Uploading combined file to S3: 100mb_combined.xlsx (size: 98.5 MB)...
✓ Successfully combined 58 chunks into 100mb_combined.xlsx
Total rows written: 578000
Combination completed in 2m15s
```

## Configuration

### Environment Variables

```bash
# Required
PROCESSING_MODE=combine                 # Enable combine mode
CHUNK_PREFIX=100mb_chunk_               # Prefix of chunk files to combine
OUTPUT_FILE=100mb_combined.xlsx         # Output filename

# S3/MinIO Connection
AWS_REGION=us-east-1
AWS_ACCESS_KEY_ID=minioadmin
AWS_SECRET_ACCESS_KEY=minioadmin
S3_BUCKET=excel-files
S3_ENDPOINT=http://localhost:9000
S3_FORCE_PATH_STYLE=true

# Optional
PROCESSING_TIMEOUT=10m                  # Timeout (default: 10 minutes)
```

### Chunk Prefix Pattern

The `CHUNK_PREFIX` should match the beginning of your chunk filenames:

**Example 1: Standard chunks**
```bash
Files: 100mb_chunk_0001.xlsx, 100mb_chunk_0002.xlsx, ...
CHUNK_PREFIX=100mb_chunk_
```

**Example 2: Custom prefix**
```bash
Files: data_part_001.xlsx, data_part_002.xlsx, ...
CHUNK_PREFIX=data_part_
```

**Example 3: With folder**
```bash
Files: output/chunks/file_001.xlsx, output/chunks/file_002.xlsx, ...
CHUNK_PREFIX=output/chunks/file_
```

## Example Usage

### Example 1: Combine Local MinIO Chunks

```bash
# Start MinIO
docker-compose up -d

# Combine chunks
export PROCESSING_MODE=combine
export CHUNK_PREFIX=100mb_chunk_
export OUTPUT_FILE=100mb_combined.xlsx
export S3_ENDPOINT=http://localhost:9000
export S3_FORCE_PATH_STYLE=true

go run main.go
```

### Example 2: Combine AWS S3 Chunks

```bash
export PROCESSING_MODE=combine
export CHUNK_PREFIX=data_chunk_
export OUTPUT_FILE=data_combined.xlsx
export AWS_REGION=us-east-1
export AWS_ACCESS_KEY_ID=your-aws-key
export AWS_SECRET_ACCESS_KEY=your-aws-secret
export S3_BUCKET=my-excel-bucket

go run main.go
```

### Example 3: Docker with Memory Limit

```bash
docker run --memory=512m -v $(pwd):/app golang:1.21 \
  -e PROCESSING_MODE=combine \
  -e CHUNK_PREFIX=100mb_chunk_ \
  -e OUTPUT_FILE=100mb_combined.xlsx \
  -e S3_ENDPOINT=http://host.docker.internal:9000 \
  -e S3_FORCE_PATH_STYLE=true \
  -e AWS_ACCESS_KEY_ID=minioadmin \
  -e AWS_SECRET_ACCESS_KEY=minioadmin \
  -e S3_BUCKET=excel-files \
  sh -c "cd /app && go run main.go"
```

## Features

### ✅ Memory-Efficient Streaming

- Uses `excelize.StreamWriter` for output
- Processes one chunk at a time
- Constant memory usage (~100-200MB)
- Can combine hundreds of chunks

### ✅ Header Management

- Automatically detects header row from first chunk
- Writes header only once in combined file
- Skips headers from subsequent chunks

### ✅ Progress Tracking

- Shows progress for each chunk
- Displays running total of rows
- Reports final statistics

### ✅ Error Handling

- Validates chunk files exist
- Handles S3 connection errors
- Provides detailed error messages

## Performance

### Benchmarks

Combining 58 chunks (~580K rows total):

| Metric | Value |
|--------|-------|
| **Processing Speed** | ~4,300 rows/sec |
| **Memory Usage** | 100-200MB (constant) |
| **Total Time** | ~2-3 minutes |
| **Network I/O** | Download chunks + Upload combined |

### Factors Affecting Speed

1. **Number of chunks**: More chunks = more S3 operations
2. **Chunk size**: Larger chunks = fewer downloads
3. **Network speed**: S3 download/upload bandwidth
4. **Row complexity**: More columns = slower processing

## Use Cases

### 1. Reconstruct Original File

After splitting a file into chunks, combine them back:

```bash
# Split
PROCESSING_MODE=chunk go run main.go

# Combine
PROCESSING_MODE=combine CHUNK_PREFIX=100mb_chunk_ go run main.go
```

### 2. Merge Distributed Processing Results

Combine results from parallel Lambda functions:

```bash
# Each Lambda creates: result_chunk_0001.xlsx, result_chunk_0002.xlsx, ...
PROCESSING_MODE=combine
CHUNK_PREFIX=result_chunk_
OUTPUT_FILE=final_results.xlsx
```

### 3. Consolidate Data from Multiple Sources

Merge data files with same structure:

```bash
PROCESSING_MODE=combine
CHUNK_PREFIX=sales_data_
OUTPUT_FILE=all_sales.xlsx
```

## Verification

### Check Combined File

```bash
# Using MinIO Client
mc ls myminio/excel-files/100mb_combined.xlsx

# Download and verify
mc cp myminio/excel-files/100mb_combined.xlsx ./
```

### Verify Row Count

The combined file should have:
- **1 header row**
- **Sum of all data rows from chunks**

Example:
- 58 chunks × 10,000 rows each = 580,000 data rows
- Combined file = 1 header + 580,000 data rows = 580,001 total rows

## Troubleshooting

### Issue: No chunk files found

**Error**: `no chunk files found with prefix: 100mb_chunk_`

**Solutions**:
1. Check `CHUNK_PREFIX` matches your files
2. Verify files exist in S3 bucket
3. Check S3 permissions
4. Ensure `S3_FORCE_PATH_STYLE=true` for MinIO

### Issue: Out of memory

**Solution**: The combiner uses streaming, so this shouldn't happen. If it does:
1. Check for memory leaks in custom code
2. Reduce `PROCESSING_TIMEOUT`
3. Process fewer chunks at a time

### Issue: Slow combination

**Solutions**:
1. Use same region for S3 bucket
2. For MinIO, run locally
3. Check network bandwidth
4. Consider combining in batches

### Issue: Duplicate headers in output

**Solution**: This shouldn't happen as headers are automatically skipped. If you see duplicates:
1. Check chunk files structure
2. Verify first row is actually a header
3. Report as a bug

## Memory Usage Details

### Streaming Architecture

```
Chunk 1 (S3) → Read Stream → Write Stream → Buffer
Chunk 2 (S3) → Read Stream → Write Stream → Buffer
...
Chunk N (S3) → Read Stream → Write Stream → Buffer
                                            ↓
                                      Combined File (S3)
```

### Memory Breakdown

| Component | Memory Usage |
|-----------|--------------|
| **Stream Reader** | ~10-20MB per chunk |
| **Stream Writer** | ~50-100MB (buffer) |
| **Go Runtime** | ~50MB |
| **Total** | ~100-200MB |

### Why It's Memory-Efficient

1. **One chunk at a time**: Never loads all chunks into memory
2. **Streaming read**: Rows processed one by one
3. **Streaming write**: Rows written immediately
4. **Periodic GC**: Garbage collection every batch
5. **No intermediate storage**: Direct chunk-to-combined transfer

## Comparison: Modes

| Feature | Read | Chunk | Combine |
|---------|------|-------|---------|
| **Input** | 1 large file | 1 large file | N chunk files |
| **Output** | No files | N chunks | 1 combined file |
| **Use Case** | Analysis | Distribution | Consolidation |
| **Memory** | 50-100MB | 100-200MB | 100-200MB |
| **Speed** | Fast (~27s) | Slow (~90s) | Medium (~2-3min) |

## Best Practices

1. **Verify chunks first**: List files before combining
2. **Use consistent naming**: Ensure chunks have sequential names
3. **Test with small sets**: Try 2-3 chunks first
4. **Monitor memory**: Use pprof to verify low usage
5. **Clean up after**: Delete chunks if no longer needed
6. **Set appropriate timeout**: Allow enough time for all chunks

## Advanced: Combining Specific Chunks

To combine only specific chunks, use a more specific prefix:

```bash
# Combine only chunks 1-10
CHUNK_PREFIX=100mb_chunk_000
# Matches: 100mb_chunk_0001.xlsx through 100mb_chunk_0009.xlsx

# Combine only chunks 10-19
CHUNK_PREFIX=100mb_chunk_001
# Matches: 100mb_chunk_0010.xlsx through 100mb_chunk_0019.xlsx
```

## Summary

Combine mode is ideal for:
- ✅ Reconstructing files after chunking
- ✅ Merging distributed processing results
- ✅ Consolidating data from multiple sources
- ✅ Memory-efficient large file assembly
- ✅ Lambda-compatible batch consolidation

The streaming approach ensures constant memory usage regardless of the number or size of chunks! 🚀
