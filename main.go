package main

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"net/http"
	_ "net/http/pprof"
	"os"
	"runtime"
	"runtime/pprof"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
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

// RowProcessor defines the interface for processing each row
type RowProcessor interface {
	ProcessSheet(rowIndex int, row []string) error
	Finalize() error
}

// ChunkUploader uploads batches as Excel chunks to S3
type ChunkUploader struct {
	processedCount int64
	batchSize      int
	batch          [][]string
	chunkNumber    int
	s3Client       *s3.Client
	ctx            context.Context
	bucketName     string
	baseFileName   string
	headerRow      []string
}

func NewChunkUploader(ctx context.Context, s3Client *s3.Client, bucketName, baseFileName string, batchSize int) *ChunkUploader {
	return &ChunkUploader{
		ctx:          ctx,
		s3Client:     s3Client,
		bucketName:   bucketName,
		baseFileName: baseFileName,
		batchSize:    batchSize,
		batch:        make([][]string, 0, batchSize),
	}
}

func (c *ChunkUploader) ProcessSheet(rowIndex int, row []string) error {
	// Save first row as header
	if rowIndex == 0 {
		c.headerRow = make([]string, len(row))
		copy(c.headerRow, row)
		return nil
	}

	c.batch = append(c.batch, row)
	c.processedCount++

	// Upload chunk when batch is full
	if len(c.batch) >= c.batchSize {
		if err := c.uploadChunk(); err != nil {
			return err
		}
	}

	// Log progress every 100k rows
	if c.processedCount%100000 == 0 {
		log.Printf("Processed %d rows | Uploaded %d chunks", c.processedCount, c.chunkNumber)
	}

	return nil
}

func (c *ChunkUploader) uploadChunk() error {
	if len(c.batch) == 0 {
		return nil
	}

	c.chunkNumber++
	startTime := time.Now()
	log.Printf("Creating chunk %d with %d rows...", c.chunkNumber, len(c.batch))

	// Create new Excel file
	f := excelize.NewFile()
	defer f.Close()

	sheetName := "Sheet1"
	index, err := f.NewSheet(sheetName)
	if err != nil {
		return fmt.Errorf("failed to create sheet: %w", err)
	}
	f.SetActiveSheet(index)

	// Write header row
	if c.headerRow != nil {
		for colIdx, value := range c.headerRow {
			cell, _ := excelize.CoordinatesToCellName(colIdx+1, 1)
			f.SetCellValue(sheetName, cell, value)
		}
	}

	// Write data rows
	for rowIdx, row := range c.batch {
		for colIdx, value := range row {
			cell, _ := excelize.CoordinatesToCellName(colIdx+1, rowIdx+2) // +2 because header is row 1
			f.SetCellValue(sheetName, cell, value)
		}
	}

	// Write to buffer
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		return fmt.Errorf("failed to write Excel to buffer: %w", err)
	}

	// Generate chunk filename
	chunkKey := fmt.Sprintf("%s_chunk_%04d.xlsx", c.baseFileName, c.chunkNumber)
	log.Printf("Uploading chunk %d to s3://%s/%s (size: %.2f MB)...",
		c.chunkNumber,
		c.bucketName,
		chunkKey,
		float64(buf.Len())/(1024*1024))

	// Upload to S3
	_, err = c.s3Client.PutObject(c.ctx, &s3.PutObjectInput{
		Bucket:      aws.String(c.bucketName),
		Key:         aws.String(chunkKey),
		Body:        bytes.NewReader(buf.Bytes()),
		ContentType: aws.String("application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"),
	})
	if err != nil {
		return fmt.Errorf("failed to upload chunk %d to S3: %w", c.chunkNumber, err)
	}

	duration := time.Since(startTime)
	log.Printf("✓ Successfully uploaded chunk %d in %s", c.chunkNumber, duration)

	// Clear the batch to free memory
	c.batch = c.batch[:0]

	// Force garbage collection periodically
	if c.processedCount%500000 == 0 {
		runtime.GC()
	}

	return nil
}

func (c *ChunkUploader) Finalize() error {
	// Upload any remaining rows in the batch
	if err := c.uploadChunk(); err != nil {
		return err
	}
	log.Printf("Total rows processed: %d", c.processedCount)
	log.Printf("Total chunks uploaded: %d", c.chunkNumber)
	return nil
}

// ExcelReader handles streaming Excel file processing
type ExcelReader struct {
	file      *excelize.File
	processor RowProcessor
}

func NewExcelReader(file *excelize.File, processor RowProcessor) *ExcelReader {
	return &ExcelReader{
		file:      file,
		processor: processor,
	}
}

// ProcessSheet processes a sheet using streaming to avoid loading all data into memory
func (r *ExcelReader) ProcessSheet(sheetName string) error {
	log.Printf("Starting to process sheet: %s", sheetName)

	// Use streaming API to read rows one by one
	rows, err := r.file.Rows(sheetName)
	if err != nil {
		return fmt.Errorf("failed to get rows iterator: %w", err)
	}
	defer rows.Close()

	rowIndex := 0
	for rows.Next() {
		row, err := rows.Columns()
		if err != nil {
			return fmt.Errorf("failed to read row %d: %w", rowIndex, err)
		}

		if err := r.processor.ProcessSheet(rowIndex, row); err != nil {
			return fmt.Errorf("failed to process row %d: %w", rowIndex, err)
		}

		rowIndex++
	}

	if err := rows.Error(); err != nil {
		return fmt.Errorf("error during row iteration: %w", err)
	}

	log.Printf("Finished processing sheet: %s with %d rows", sheetName, rowIndex)
	return nil
}

// DownloadAndProcessExcel downloads an Excel file from S3 and processes it with streaming
func DownloadAndProcessExcel(ctx context.Context, s3Client *s3.Client, cfg Config, processor RowProcessor) error {
	startTime := time.Now()
	log.Printf("Starting Excel processing from S3")

	log.Printf("Connected to S3 (Region: %s, Endpoint: %s)", cfg.Region, cfg.Endpoint)

	// Check if bucket exists (optional, HeadBucket)
	_, err := s3Client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: aws.String(cfg.BucketName),
	})
	if err != nil {
		return fmt.Errorf("bucket %s does not exist or is not accessible: %w", cfg.BucketName, err)
	}

	log.Printf("Downloading file: %s from bucket: %s", cfg.ObjectName, cfg.BucketName)

	// Get object from S3 with streaming
	result, err := s3Client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(cfg.BucketName),
		Key:    aws.String(cfg.ObjectName),
	})
	if err != nil {
		return fmt.Errorf("failed to get object: %w", err)
	}
	defer result.Body.Close()

	// Get object size from metadata
	var fileSize int64
	if result.ContentLength != nil {
		fileSize = *result.ContentLength
		log.Printf("File size: %.2f MB", float64(fileSize)/(1024*1024))
	}

	// Open Excel file from the stream
	// excelize.OpenReader reads from io.Reader, which is memory-efficient
	file, err := excelize.OpenReader(result.Body)
	if err != nil {
		return fmt.Errorf("failed to open Excel file: %w", err)
	}
	defer file.Close()

	log.Printf("Excel file opened successfully")

	// Get list of sheets
	sheets := file.GetSheetList()
	log.Printf("Found %d sheet(s): %v", len(sheets), sheets)

	// ProcessSheet each sheet
	reader := NewExcelReader(file, processor)
	for _, sheetName := range sheets {
		if err := reader.ProcessSheet(sheetName); err != nil {
			return fmt.Errorf("failed to process sheet %s: %w", sheetName, err)
		}
	}

	// Finalize processing
	if err = processor.Finalize(); err != nil {
		return fmt.Errorf("failed to finalize processing: %w", err)
	}

	duration := time.Since(startTime)
	log.Printf("Excel processing completed in %s", duration)

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
		Region:   getEnv("AWS_REGION", "us-east-1"),
		Endpoint: getEnv("S3_ENDPOINT", "http://host.docker.internal:9000"), // Empty for AWS S3, set for MinIO host.docker.internal
		// Endpoint:        getEnv("S3_ENDPOINT", "http://localhost:9000"), // Empty for AWS S3, set for MinIO host.docker.internal
		AccessKeyID:     getEnv("AWS_ACCESS_KEY_ID", "minioadmin"),
		SecretAccessKey: getEnv("AWS_SECRET_ACCESS_KEY", "minioadmin"),
		BucketName:      getEnv("S3_BUCKET", "excel-files"),
		ObjectName:      getEnv("EXCEL_FILE_NAME", "100mb.xlsx"),
		ForcePathStyle:  getEnv("S3_FORCE_PATH_STYLE", "true") == "true", // true for MinIO
	}

	// Get processing mode
	chunkSize := 10000 // Default: 10k rows per chunk

	log.Printf("Configuration: Region=%s, Endpoint=%s, Bucket=%s, File=%s",
		config.Region, config.Endpoint, config.BucketName, config.ObjectName)

	// Create context with timeout (adjust based on Lambda timeout)
	// For Lambda: use context from Lambda handler or set shorter timeout
	timeout := getEnv("PROCESSING_TIMEOUT", "5m") // Default 5 minutes
	timeoutDuration, err := time.ParseDuration(timeout)
	if err != nil {
		timeoutDuration = 5 * time.Minute
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeoutDuration)
	defer cancel()

	var processor RowProcessor

	// Mode: Create chunks and upload to S3
	log.Printf("Mode: Split file into chunks and upload to S3")
	log.Printf("Chunk size: %d rows per file", chunkSize)

	// Initialize S3 client for uploading chunks
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

	// Extract base filename without extension
	baseFileName := strings.TrimSuffix(config.ObjectName, ".xlsx")
	processor = NewChunkUploader(ctx, s3Client, config.BucketName, baseFileName, chunkSize)

	// ProcessSheet the Excel file
	if err := DownloadAndProcessExcel(ctx, s3Client, config, processor); err != nil {
		log.Fatalf("Error processing Excel file: %v", err)
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
