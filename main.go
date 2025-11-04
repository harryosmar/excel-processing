package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	_ "net/http/pprof"
	"os"
	"runtime"
	"runtime/pprof"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/xuri/excelize/v2"
)

// Config holds the S3 configuration
type Config struct {
	Region          string
	Endpoint        string // Optional: for S3-compatible services like MinIO
	AccessKeyID     string
	SecretAccessKey string
	BucketName      string
	ObjectName      string
	ForcePathStyle  bool // Set to true for MinIO/S3-compatible services
}

// ChunkCombiner combines multiple chunk files from S3 into one Excel file
type ChunkCombiner struct {
	s3Client        *s3.Client
	ctx             context.Context
	bucketName      string
	chunkPrefix     string
	outputFileName  string
	streamWriter    *excelize.StreamWriter
	file            *excelize.File
	currentRow      int
	currentSheet    int
	headerWritten   bool
	maxRowsPerSheet int
}

// NewChunkCombiner creates a new chunk combiner
func NewChunkCombiner(ctx context.Context, s3Client *s3.Client, bucketName, chunkPrefix, outputFileName string) (*ChunkCombiner, error) {
	// Create new Excel file for output
	f := excelize.NewFile()
	sheetName := "Sheet1"

	// Create stream writer for memory-efficient writing
	streamWriter, err := f.NewStreamWriter(sheetName)
	if err != nil {
		return nil, fmt.Errorf("failed to create stream writer: %w", err)
	}

	return &ChunkCombiner{
		s3Client:        s3Client,
		ctx:             ctx,
		bucketName:      bucketName,
		chunkPrefix:     chunkPrefix,
		outputFileName:  outputFileName,
		file:            f,
		streamWriter:    streamWriter,
		currentRow:      1,
		currentSheet:    1,
		headerWritten:   false,
		maxRowsPerSheet: 1048576, // Excel's maximum rows per sheet
	}, nil
}

// CombineChunks combines all chunk files from S3 into one Excel file
func (c *ChunkCombiner) CombineChunks() error {
	startTime := time.Now()
	log.Printf("Starting chunk combination from S3...")
	log.Printf("Bucket: %s, Prefix: %s", c.bucketName, c.chunkPrefix)

	// List all chunk files
	listInput := &s3.ListObjectsV2Input{
		Bucket: aws.String(c.bucketName),
		Prefix: aws.String(c.chunkPrefix),
	}

	result, err := c.s3Client.ListObjectsV2(c.ctx, listInput)
	if err != nil {
		return fmt.Errorf("failed to list chunk files: %w", err)
	}

	if len(result.Contents) == 0 {
		return fmt.Errorf("no chunk files found with prefix: %s", c.chunkPrefix)
	}

	log.Printf("Found %d chunk files to combine", len(result.Contents))

	// Process each chunk file
	totalRows := 0
	for idx, obj := range result.Contents {
		chunkKey := *obj.Key
		log.Printf("Processing chunk %d/%d: %s", idx+1, len(result.Contents), chunkKey)

		rowsProcessed, err := c.processChunkFile(chunkKey)
		if err != nil {
			return fmt.Errorf("failed to process chunk %s: %w", chunkKey, err)
		}

		totalRows += rowsProcessed
		log.Printf("✓ Processed %s: %d rows (total: %d)", chunkKey, rowsProcessed, totalRows)

		// Force GC after each chunk to free memory immediately
		runtime.GC()
	}

	// Flush stream writer
	if err := c.streamWriter.Flush(); err != nil {
		return fmt.Errorf("failed to flush stream writer: %w", err)
	}

	log.Printf("Streaming combined file to S3 using multipart upload...")

	// Create a pipe for streaming upload
	pipeReader, pipeWriter := io.Pipe()

	// Upload in a goroutine using multipart upload
	uploadErr := make(chan error, 1)
	go func() {
		defer pipeReader.Close()
		
		// Use S3 manager for multipart upload (handles chunking automatically)
		uploader := manager.NewUploader(c.s3Client, func(u *manager.Uploader) {
			u.PartSize = 10 * 1024 * 1024 // 10MB parts
			u.Concurrency = 1             // Sequential upload to save memory
		})
		
		_, err := uploader.Upload(c.ctx, &s3.PutObjectInput{
			Bucket:      aws.String(c.bucketName),
			Key:         aws.String(c.outputFileName),
			Body:        pipeReader,
			ContentType: aws.String("application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"),
		})
		uploadErr <- err
	}()

	// Write Excel file directly to pipe (streaming)
	log.Printf("Writing Excel file to stream...")
	if err := c.file.Write(pipeWriter); err != nil {
		pipeWriter.Close()
		return fmt.Errorf("failed to write Excel to stream: %w", err)
	}
	pipeWriter.Close()

	// Wait for upload to complete
	log.Printf("Waiting for upload to complete...")
	if err := <-uploadErr; err != nil {
		return fmt.Errorf("failed to upload combined file: %w", err)
	}
	
	log.Printf("✓ Upload completed successfully")

	duration := time.Since(startTime)
	log.Printf("✓ Successfully combined %d chunks into %s", len(result.Contents), c.outputFileName)
	log.Printf("Total rows written: %d", totalRows)
	log.Printf("Combination completed in %s", duration)

	return nil
}

// processChunkFile downloads and processes a single chunk file
func (c *ChunkCombiner) processChunkFile(chunkKey string) (int, error) {
	// Download chunk from S3
	getObjectInput := &s3.GetObjectInput{
		Bucket: aws.String(c.bucketName),
		Key:    aws.String(chunkKey),
	}

	result, err := c.s3Client.GetObject(c.ctx, getObjectInput)
	if err != nil {
		return 0, fmt.Errorf("failed to download chunk: %w", err)
	}
	defer result.Body.Close()

	// Open Excel file from stream
	chunkFile, err := excelize.OpenReader(result.Body)
	if err != nil {
		return 0, fmt.Errorf("failed to open chunk Excel file: %w", err)
	}
	// Explicitly close chunk file to free memory immediately
	defer func() {
		if err := chunkFile.Close(); err != nil {
			log.Printf("Warning: failed to close chunk file: %v", err)
		}
	}()

	// Get first sheet
	sheets := chunkFile.GetSheetList()
	if len(sheets) == 0 {
		return 0, fmt.Errorf("no sheets found in chunk file")
	}

	sheetName := sheets[0]
	rows, err := chunkFile.Rows(sheetName)
	if err != nil {
		return 0, fmt.Errorf("failed to get rows: %w", err)
	}
	defer rows.Close()

	rowCount := 0
	for rows.Next() {
		row, err := rows.Columns()
		if err != nil {
			return 0, fmt.Errorf("failed to read row: %w", err)
		}

		// Skip header row if already written
		if rowCount == 0 {
			if !c.headerWritten {
				// Write header row
				if err := c.writeRow(row); err != nil {
					return 0, err
				}
				c.headerWritten = true
			}
			rowCount++
			continue
		}

		// Write data row
		if err := c.writeRow(row); err != nil {
			return 0, err
		}
		rowCount++
	}

	// Ensure rows iterator is closed before returning
	if err := rows.Close(); err != nil {
		log.Printf("Warning: failed to close rows iterator: %v", err)
	}

	return rowCount - 1, nil // Subtract header row
}

// writeRow writes a single row using stream writer
func (c *ChunkCombiner) writeRow(row []string) error {
	// Check if we need to create a new sheet
	if c.currentRow >= c.maxRowsPerSheet {
		if err := c.createNewSheet(); err != nil {
			return err
		}
	}

	// Create cell slice directly for stream writer (single allocation)
	cellData := make([]interface{}, len(row))
	for i, value := range row {
		cellData[i] = excelize.Cell{Value: value}
	}

	if err := c.streamWriter.SetRow(fmt.Sprintf("A%d", c.currentRow), cellData); err != nil {
		return fmt.Errorf("failed to write row %d: %w", c.currentRow, err)
	}

	c.currentRow++
	return nil
}

// createNewSheet creates a new sheet when row limit is reached
func (c *ChunkCombiner) createNewSheet() error {
	// Flush current sheet
	if err := c.streamWriter.Flush(); err != nil {
		return fmt.Errorf("failed to flush current sheet: %w", err)
	}

	c.currentSheet++
	sheetName := fmt.Sprintf("Sheet%d", c.currentSheet)

	log.Printf("Creating new sheet: %s (row limit reached)", sheetName)

	// Create new sheet
	index, err := c.file.NewSheet(sheetName)
	if err != nil {
		return fmt.Errorf("failed to create new sheet: %w", err)
	}
	c.file.SetActiveSheet(index)

	// Create new stream writer for the new sheet
	streamWriter, err := c.file.NewStreamWriter(sheetName)
	if err != nil {
		return fmt.Errorf("failed to create stream writer for new sheet: %w", err)
	}

	c.streamWriter = streamWriter
	c.currentRow = 1
	c.headerWritten = false // Write header again for new sheet

	return nil
}

// Close closes the combiner and cleans up resources
func (c *ChunkCombiner) Close() error {
	if c.file != nil {
		return c.file.Close()
	}
	return nil
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Println("Excel Processing Application Started")

	// Enable pprof profiling
	enablePprof := getEnv("ENABLE_PPROF", "true") == "true"
	pprofPort := getEnv("PPROF_PORT", "6060")
	cpuProfile := getEnv("CPU_PROFILE", "cpu.prof")
	memProfile := getEnv("MEM_PROFILE", "mem.prof")

	if enablePprof {
		// Start pprof HTTP server
		go func() {
			log.Printf("Starting pprof server on http://localhost:%s/debug/pprof/", pprofPort)
			if err := http.ListenAndServe(":"+pprofPort, nil); err != nil {
				log.Printf("pprof server error: %v", err)
			}
		}()
	}

	// CPU profiling
	if cpuProfile != "" {
		f, err := os.Create(cpuProfile)
		if err != nil {
			log.Fatalf("Could not create CPU profile: %v", err)
		}
		defer f.Close()
		if err := pprof.StartCPUProfile(f); err != nil {
			log.Fatalf("Could not start CPU profile: %v", err)
		}
		defer pprof.StopCPUProfile()
		log.Printf("CPU profiling enabled, writing to: %s", cpuProfile)
	}

	// Load configuration from environment variables
	config := Config{
		Region: getEnv("AWS_REGION", "us-east-1"),
		// Endpoint: getEnv("S3_ENDPOINT", "http://host.docker.internal:9000"), // Empty for AWS S3, set for MinIO host.docker.internal
		Endpoint:        getEnv("S3_ENDPOINT", "http://localhost:9000"), // Empty for AWS S3, set for MinIO host.docker.internal
		AccessKeyID:     getEnv("AWS_ACCESS_KEY_ID", "minioadmin"),
		SecretAccessKey: getEnv("AWS_SECRET_ACCESS_KEY", "minioadmin"),
		BucketName:      getEnv("S3_BUCKET", "excel-files"),
		ObjectName:      getEnv("EXCEL_FILE_NAME", "100mb.xlsx"),
		ForcePathStyle:  getEnv("S3_FORCE_PATH_STYLE", "true") == "true", // true for MinIO
	}

	// Get combine mode configuration
	chunkPrefix := getEnv("CHUNK_PREFIX", "100mb_chunk_")      // Prefix for chunk files
	outputFile := getEnv("OUTPUT_FILE", "100mb_combined.xlsx") // Output file for combine mode

	log.Printf("Configuration: Region=%s, Endpoint=%s, Bucket=%s",
		config.Region, config.Endpoint, config.BucketName)
	log.Printf("Chunk prefix: %s", chunkPrefix)
	log.Printf("Output file: %s", outputFile)

	// Create context with timeout
	timeout := getEnv("PROCESSING_TIMEOUT", "10m") // Default 10 minutes
	timeoutDuration, err := time.ParseDuration(timeout)
	if err != nil {
		timeoutDuration = 10 * time.Minute
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeoutDuration)
	defer cancel()

	// Initialize S3 client
	var awsCfg aws.Config

	if config.Endpoint != "" {
		customResolver := aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...interface{}) (aws.Endpoint, error) {
			return aws.Endpoint{
				URL:               config.Endpoint,
				HostnameImmutable: true,
				SigningRegion:     config.Region,
			}, nil
		})

		awsCfg, err = awsconfig.LoadDefaultConfig(ctx,
			awsconfig.WithRegion(config.Region),
			awsconfig.WithEndpointResolverWithOptions(customResolver),
			awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
				config.AccessKeyID,
				config.SecretAccessKey,
				"",
			)),
		)
	} else {
		awsCfg, err = awsconfig.LoadDefaultConfig(ctx,
			awsconfig.WithRegion(config.Region),
			awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
				config.AccessKeyID,
				config.SecretAccessKey,
				"",
			)),
		)
	}

	if err != nil {
		log.Fatalf("Failed to load AWS config: %v", err)
	}

	s3Client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.UsePathStyle = config.ForcePathStyle
	})

	// Combine chunks into one file
	log.Printf("Starting chunk combination...")

	combiner, err := NewChunkCombiner(ctx, s3Client, config.BucketName, chunkPrefix, outputFile)
	if err != nil {
		log.Fatalf("Failed to create chunk combiner: %v", err)
	}
	defer combiner.Close()

	if err := combiner.CombineChunks(); err != nil {
		log.Fatalf("Error combining chunks: %v", err)
	}

	// Memory profiling
	if memProfile != "" {
		f, err := os.Create(memProfile)
		if err != nil {
			log.Fatalf("Could not create memory profile: %v", err)
		}
		defer f.Close()
		runtime.GC() // get up-to-date statistics
		if err := pprof.WriteHeapProfile(f); err != nil {
			log.Fatalf("Could not write memory profile: %v", err)
		}
		log.Printf("Memory profile written to: %s", memProfile)
	}

	log.Println("Application finished successfully")
}
