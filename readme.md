# Excel Chunk Combiner

Memory-efficient tool to combine multiple Excel chunk files from S3 into one large Excel file using streaming.

## Features

- ✅ **Memory-efficient streaming**: Constant ~100-200MB memory usage
- ✅ **S3 integration**: Reads chunks from and writes combined file to S3
- ✅ **Smart header handling**: Automatically skips duplicate headers
- ✅ **Progress tracking**: Real-time progress for each chunk
- ✅ **Lambda-compatible**: Works in AWS Lambda with 512MB memory

## Quick Start

### 1. Configure Environment

```bash
cp .env.example .env
# Edit .env with your settings
```

### 2. Run Locally

```bash
export CHUNK_PREFIX=100mb_chunk_
export OUTPUT_FILE=100mb_combined.xlsx
export S3_ENDPOINT=http://localhost:9000
export S3_FORCE_PATH_STYLE=true
export AWS_ACCESS_KEY_ID=minioadmin
export AWS_SECRET_ACCESS_KEY=minioadmin
export S3_BUCKET=excel-files

go run main.go
```

### 3. Run with Docker (Memory Limited)

```bash
docker run --memory=512m -v $(pwd):/app golang:1.21 \
  -e CHUNK_PREFIX=100mb_chunk_ \
  -e OUTPUT_FILE=100mb_combined.xlsx \
  -e S3_ENDPOINT=http://host.docker.internal:9000 \
  -e S3_FORCE_PATH_STYLE=true \
  -e AWS_ACCESS_KEY_ID=minioadmin \
  -e AWS_SECRET_ACCESS_KEY=minioadmin \
  -e S3_BUCKET=excel-files \
  sh -c "cd /app && go run main.go"
```

## Configuration

| Variable | Description | Default |
|----------|-------------|---------|
| `CHUNK_PREFIX` | Prefix of chunk files to combine | `100mb_chunk_` |
| `OUTPUT_FILE` | Output filename | `100mb_combined.xlsx` |
| `S3_BUCKET` | S3 bucket name | `excel-files` |
| `S3_ENDPOINT` | S3 endpoint (for MinIO) | `http://localhost:9000` |
| `S3_FORCE_PATH_STYLE` | Use path-style URLs (MinIO) | `true` |
| `AWS_REGION` | AWS region | `us-east-1` |
| `AWS_ACCESS_KEY_ID` | AWS access key | - |
| `AWS_SECRET_ACCESS_KEY` | AWS secret key | - |
| `PROCESSING_TIMEOUT` | Processing timeout | `10m` |

## How It Works

```
1. List all files matching CHUNK_PREFIX in S3
2. Download each chunk (streaming)
3. Read rows from chunk (streaming)
4. Write rows to combined file (streaming)
5. Skip duplicate headers
6. Upload combined file to S3
```

## Example

**Input**: 58 chunk files in S3
- `100mb_chunk_0001.xlsx` (10,000 rows)
- `100mb_chunk_0002.xlsx` (10,000 rows)
- ...
- `100mb_chunk_0058.xlsx` (8,000 rows)

**Output**: 1 combined file
- `100mb_combined.xlsx` (578,000 rows + 1 header)

**Memory**: ~100-200MB (constant)

## Performance

- **Speed**: ~4,300 rows/sec
- **Memory**: 100-200MB (constant)
- **Time**: ~2-3 minutes for 580K rows

## MinIO Setup

```bash
docker-compose up -d
```

Access MinIO console: http://localhost:9001
- Username: `minioadmin`
- Password: `minioadmin`

## Profiling

Enable pprof for memory/CPU profiling:

```bash
export ENABLE_PPROF=true
export CPU_PROFILE=cpu.prof
export MEM_PROFILE=mem.prof

go run main.go

# Analyze
go tool pprof -http=:8080 mem.prof
```

## Documentation

- [COMBINE_MODE.md](COMBINE_MODE.md) - Detailed usage guide
- [PPROF_USAGE.md](PPROF_USAGE.md) - Profiling guide

## Test Data

Sample 100MB Excel file: https://examplefile.com/document/xlsx/100-mb-xlsx

## License

MIT