# Chunk Mode Guide

Split large Excel files into smaller chunks and upload them to S3.

## Overview

**Chunk mode** reads a large Excel file from S3 and splits it into smaller Excel files (chunks), then uploads each chunk back to S3 in the same bucket.

## How It Works

```
Large Excel File (S3) → Stream Read → Split into Chunks → Create Excel Files → Upload to S3
```

### Process Flow

1. Downloads large Excel file from S3 using streaming
2. Reads rows one by one (memory-efficient)
3. Accumulates rows into chunks (default: 10,000 rows per chunk)
4. Creates a new Excel file for each chunk
5. Uploads each chunk file to S3
6. Clears memory and continues with next chunk

## Quick Start

### Enable Chunk Mode

```bash
export PROCESSING_MODE=chunk
export S3_ENDPOINT=http://localhost:9000
export S3_FORCE_PATH_STYLE=true
export AWS_ACCESS_KEY_ID=minioadmin
export AWS_SECRET_ACCESS_KEY=minioadmin
export S3_BUCKET=excel-files
export EXCEL_FILE_NAME=100mb.xlsx

go run main.go
```

### Output Files

Chunks are named using this pattern:

```
{original_filename}_chunk_{NNNN}.xlsx
```

**Example:**

Input: `100mb.xlsx` (1,174,859 rows)

Output:
- `100mb_chunk_0001.xlsx` (10,000 rows + header)
- `100mb_chunk_0002.xlsx` (10,000 rows + header)
- `100mb_chunk_0003.xlsx` (10,000 rows + header)
- ...
- `100mb_chunk_0117.xlsx` (10,000 rows + header)
- `100mb_chunk_0118.xlsx` (4,859 rows + header)

Total: **118 chunk files**

## Configuration

### Environment Variables

```bash
# Required
PROCESSING_MODE=chunk              # Enable chunk mode
S3_BUCKET=excel-files              # Source and destination bucket
EXCEL_FILE_NAME=100mb.xlsx         # Input file name

# S3/MinIO Connection
AWS_REGION=us-east-1
AWS_ACCESS_KEY_ID=minioadmin
AWS_SECRET_ACCESS_KEY=minioadmin
S3_ENDPOINT=http://localhost:9000
S3_FORCE_PATH_STYLE=true

# Optional
PROCESSING_TIMEOUT=5m              # Timeout (default: 5 minutes)
```

### Chunk Size

The chunk size is set in the code (default: 10,000 rows per file):

```go
chunkSize := 10000  // Rows per chunk file
```

To change it, modify line 449 in `main.go`.

## Example Usage

### Example 1: Local MinIO

```bash
# Start MinIO
docker-compose up -d

# Upload a large Excel file
# (Use MinIO console at http://localhost:9001)

# Run chunking
export PROCESSING_MODE=chunk
export S3_ENDPOINT=http://localhost:9000
export S3_FORCE_PATH_STYLE=true
export AWS_ACCESS_KEY_ID=minioadmin
export AWS_SECRET_ACCESS_KEY=minioadmin
export S3_BUCKET=excel-files
export EXCEL_FILE_NAME=100mb.xlsx

go run main.go
```

### Example 2: AWS S3

```bash
export PROCESSING_MODE=chunk
export AWS_REGION=us-east-1
export AWS_ACCESS_KEY_ID=your-aws-key
export AWS_SECRET_ACCESS_KEY=your-aws-secret
export S3_BUCKET=my-excel-bucket
export EXCEL_FILE_NAME=large-data.xlsx

go run main.go
```

### Example 3: Docker with Memory Limit

```bash
# Test with 512MB memory limit (Lambda-like)
docker run --memory=512m -v $(pwd):/app golang:1.21 \
  -e PROCESSING_MODE=chunk \
  -e S3_ENDPOINT=http://host.docker.internal:9000 \
  -e S3_FORCE_PATH_STYLE=true \
  -e AWS_ACCESS_KEY_ID=minioadmin \
  -e AWS_SECRET_ACCESS_KEY=minioadmin \
  -e S3_BUCKET=excel-files \
  -e EXCEL_FILE_NAME=100mb.xlsx \
  sh -c "cd /app && go run main.go"
```

## Log Output

```
2025/11/03 01:00:00 Excel Processing Application Started
2025/11/03 01:00:00 Configuration: Region=us-east-1, Endpoint=http://localhost:9000, Bucket=excel-files, File=100mb.xlsx, Mode=chunk
2025/11/03 01:00:00 Mode: Split file into chunks and upload to S3
2025/11/03 01:00:00 Chunk size: 10000 rows per file
2025/11/03 01:00:00 Starting Excel processing from S3
2025/11/03 01:00:00 Connected to S3 (Region: us-east-1, Endpoint: http://localhost:9000)
2025/11/03 01:00:00 Downloading file: 100mb.xlsx from bucket: excel-files
2025/11/03 01:00:00 File size: 101.23 MB
2025/11/03 01:00:00 Excel file opened successfully
2025/11/03 01:00:00 Found 1 sheet(s): [Worksheet]
2025/11/03 01:00:00 Starting to process sheet: Worksheet
2025/11/03 01:00:01 Creating chunk 1 with 10000 rows...
2025/11/03 01:00:01 Uploading chunk 1 to s3://excel-files/100mb_chunk_0001.xlsx (size: 2.45 MB)...
2025/11/03 01:00:01 ✓ Successfully uploaded chunk 1 in 1.2s
2025/11/03 01:00:02 Creating chunk 2 with 10000 rows...
2025/11/03 01:00:02 Uploading chunk 2 to s3://excel-files/100mb_chunk_0002.xlsx (size: 2.45 MB)...
2025/11/03 01:00:02 ✓ Successfully uploaded chunk 2 in 1.1s
...
2025/11/03 01:00:45 Processed 100000 rows | Uploaded 10 chunks
...
2025/11/03 01:01:30 Creating chunk 118 with 4859 rows...
2025/11/03 01:01:30 Uploading chunk 118 to s3://excel-files/100mb_chunk_0118.xlsx (size: 1.19 MB)...
2025/11/03 01:01:30 ✓ Successfully uploaded chunk 118 in 0.8s
2025/11/03 01:01:30 Total rows processed: 1174859
2025/11/03 01:01:30 Total chunks uploaded: 118
2025/11/03 01:01:30 Excel processing completed in 90.5s
2025/11/03 01:01:30 Application finished successfully
```

## Performance

### Benchmarks

Tested with 1.17M row Excel file (~100MB):

| Metric | Value |
|--------|-------|
| **Processing Speed** | ~13,000 rows/sec |
| **Chunk Creation** | ~1-2 seconds per chunk |
| **Upload Speed** | ~1 second per chunk (local MinIO) |
| **Memory Usage** | 100-200MB (constant) |
| **Total Time** | ~90 seconds for 118 chunks |

### Chunk Size Recommendations

| Chunk Size | Chunks (1M rows) | File Size | Use Case |
|------------|------------------|-----------|----------|
| **5,000** | 200 | ~1.2 MB | Small, many files |
| **10,000** | 100 | ~2.5 MB | **Recommended** |
| **25,000** | 40 | ~6 MB | Fewer, larger files |
| **50,000** | 20 | ~12 MB | Large chunks |

## Use Cases

### 1. Data Distribution

Split large dataset for distribution to multiple teams:

```bash
PROCESSING_MODE=chunk
# Each team gets manageable chunk files
```

### 2. Parallel Processing

Create chunks for parallel Lambda processing:

```bash
PROCESSING_MODE=chunk
# Process each chunk independently in parallel
```

### 3. Data Migration

Split large files for easier migration:

```bash
PROCESSING_MODE=chunk
# Migrate chunks one at a time
```

### 4. Batch Processing

Process data in smaller batches:

```bash
PROCESSING_MODE=chunk
# Each chunk can be processed by different workers
```

## Verifying Chunks

### Using MinIO Console

1. Open http://localhost:9001
2. Login with credentials
3. Navigate to your bucket
4. You should see all chunk files listed

### Using MinIO Client (mc)

```bash
# List chunks
mc ls myminio/excel-files/ | grep chunk

# Count chunks
mc ls myminio/excel-files/ | grep chunk | wc -l

# Download a chunk
mc cp myminio/excel-files/100mb_chunk_0001.xlsx ./

# Get total size of all chunks
mc du myminio/excel-files/ --recursive | grep chunk
```

### Using AWS CLI

```bash
# List chunks
aws s3 ls s3://excel-files/ --prefix 100mb_chunk

# Count chunks
aws s3 ls s3://excel-files/ --prefix 100mb_chunk | wc -l

# Download a chunk
aws s3 cp s3://excel-files/100mb_chunk_0001.xlsx ./

# Get total size
aws s3 ls s3://excel-files/ --prefix 100mb_chunk --recursive --human-readable --summarize
```

## Chunk File Structure

Each chunk file contains:

- **Header row**: Copied from the original file (row 1)
- **Data rows**: The chunk of rows (e.g., 10,000 rows)
- **Format**: Standard Excel (.xlsx) format
- **Sheets**: Single sheet named "Sheet1"

## Memory Efficiency

- Only one chunk is in memory at a time
- Constant memory usage (~100-200MB)
- Can process files with millions of rows
- Periodic garbage collection
- Works in Lambda with 512MB memory

## Troubleshooting

### Issue: Chunks not appearing in S3

**Solutions:**
1. Check S3_FORCE_PATH_STYLE=true for MinIO
2. Verify bucket permissions
3. Check logs for upload errors
4. Ensure S3_ENDPOINT is correct

### Issue: Out of memory

**Solution**: Reduce chunk size in `main.go`:

```go
chunkSize := 5000  // Smaller chunks
```

### Issue: Slow uploads

**Solutions:**
1. Use same region for S3 bucket
2. For MinIO, run locally
3. Increase network bandwidth
4. Check S3 endpoint connectivity

### Issue: Duplicate chunks

**Solution**: Chunks are numbered sequentially. If you re-run, it will create new chunks with same names (overwriting old ones).

## Comparison: Read vs Chunk Mode

| Feature | Read Mode | Chunk Mode |
|---------|-----------|------------|
| **Output** | No files created | Multiple Excel chunks |
| **Use Case** | Data analysis, ETL | Distribution, parallel processing |
| **Memory** | 50-100MB | 100-200MB |
| **Speed** | Faster (~27s) | Slower (~90s with uploads) |
| **Storage** | No additional storage | Requires output storage |
| **Lambda Compatible** | ✅ Yes | ✅ Yes |

## Best Practices

1. **Test with small files first**: Verify configuration works
2. **Monitor S3 costs**: Chunking creates many files
3. **Clean up old chunks**: Remove chunks after processing
4. **Use descriptive filenames**: Original filename is preserved in chunk name
5. **Set appropriate timeout**: Allow enough time for all chunks to upload
6. **Disable profiling in production**: Set ENABLE_PPROF=false

## Summary

Chunk mode is ideal for:
- ✅ Breaking large files into manageable pieces
- ✅ Distributing data to multiple teams/workers
- ✅ Parallel processing workflows
- ✅ Lambda-based batch processing
- ✅ Data migration scenarios
- ✅ Memory-constrained environments

The streaming approach ensures memory-efficient processing regardless of input file size!
