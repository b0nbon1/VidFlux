package stream

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	gonanoid "github.com/matoous/go-nanoid/v2"
)

// UploadSession tracks the state of a resumable upload
type UploadSession struct {
	ID            string    `json:"id"`
	Filename      string    `json:"filename"`
	TotalSize     int64     `json:"total_size"`
	ChunkSize     int64     `json:"chunk_size"`
	UploadedBytes int64     `json:"uploaded_bytes"`
	TotalChunks   int       `json:"total_chunks"`
	UploadedChunks []int    `json:"uploaded_chunks"`
	Status        string    `json:"status"` // "pending", "uploading", "processing", "complete", "error"
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
	TempPath      string    `json:"-"`
	FinalPath     string    `json:"-"`
	VideoID       string    `json:"video_id,omitempty"`
}

// In-memory session store (in production, use Redis or database)
var (
	uploadSessions = make(map[string]*UploadSession)
	sessionMutex   sync.RWMutex
)

const (
	defaultChunkSize = 5 * 1024 * 1024 // 5MB chunks
	uploadTempDir    = "./static/uploads/temp"
)

// InitUploadRequest represents the request to initialize an upload
type InitUploadRequest struct {
	Filename  string `json:"filename"`
	TotalSize int64  `json:"total_size"`
	ChunkSize int64  `json:"chunk_size,omitempty"`
}

// InitUpload initializes a new resumable upload session
func InitUpload(c *fiber.Ctx) error {
	var req InitUploadRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	if req.Filename == "" || req.TotalSize <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Filename and total_size are required",
		})
	}

	// Generate unique upload ID
	uploadID, err := gonanoid.Generate("0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ", 12)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to generate upload ID",
		})
	}

	chunkSize := req.ChunkSize
	if chunkSize <= 0 {
		chunkSize = defaultChunkSize
	}

	totalChunks := int((req.TotalSize + chunkSize - 1) / chunkSize)

	// Create temp directory for this upload
	tempPath := filepath.Join(uploadTempDir, uploadID)
	if err := os.MkdirAll(tempPath, 0755); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to create upload directory",
		})
	}

	session := &UploadSession{
		ID:             uploadID,
		Filename:       req.Filename,
		TotalSize:      req.TotalSize,
		ChunkSize:      chunkSize,
		UploadedBytes:  0,
		TotalChunks:    totalChunks,
		UploadedChunks: []int{},
		Status:         "pending",
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
		TempPath:       tempPath,
	}

	sessionMutex.Lock()
	uploadSessions[uploadID] = session
	sessionMutex.Unlock()

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"upload_id":    uploadID,
		"chunk_size":   chunkSize,
		"total_chunks": totalChunks,
	})
}

// UploadChunk handles uploading a single chunk
func UploadChunk(c *fiber.Ctx) error {
	uploadID := c.Params("uploadId")
	chunkIndexStr := c.Params("chunkIndex")

	chunkIndex, err := strconv.Atoi(chunkIndexStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid chunk index",
		})
	}

	sessionMutex.RLock()
	session, exists := uploadSessions[uploadID]
	sessionMutex.RUnlock()

	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Upload session not found",
		})
	}

	if chunkIndex < 0 || chunkIndex >= session.TotalChunks {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Chunk index out of range",
		})
	}

	// Check if chunk already uploaded
	for _, idx := range session.UploadedChunks {
		if idx == chunkIndex {
			return c.Status(fiber.StatusOK).JSON(fiber.Map{
				"message":        "Chunk already uploaded",
				"chunk_index":    chunkIndex,
				"uploaded_bytes": session.UploadedBytes,
			})
		}
	}

	// Get the chunk data from the request body
	chunkData := c.Body()
	if len(chunkData) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Empty chunk data",
		})
	}

	// Save chunk to temp file
	chunkPath := filepath.Join(session.TempPath, fmt.Sprintf("chunk_%d", chunkIndex))
	if err := os.WriteFile(chunkPath, chunkData, 0644); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to save chunk",
		})
	}

	// Update session
	sessionMutex.Lock()
	session.UploadedChunks = append(session.UploadedChunks, chunkIndex)
	session.UploadedBytes += int64(len(chunkData))
	session.Status = "uploading"
	session.UpdatedAt = time.Now()
	sessionMutex.Unlock()

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message":         "Chunk uploaded successfully",
		"chunk_index":     chunkIndex,
		"uploaded_bytes":  session.UploadedBytes,
		"uploaded_chunks": len(session.UploadedChunks),
		"total_chunks":    session.TotalChunks,
	})
}

// GetUploadStatus returns the current status of an upload
func GetUploadStatus(c *fiber.Ctx) error {
	uploadID := c.Params("uploadId")

	sessionMutex.RLock()
	session, exists := uploadSessions[uploadID]
	sessionMutex.RUnlock()

	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Upload session not found",
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"upload_id":       session.ID,
		"filename":        session.Filename,
		"total_size":      session.TotalSize,
		"uploaded_bytes":  session.UploadedBytes,
		"total_chunks":    session.TotalChunks,
		"uploaded_chunks": session.UploadedChunks,
		"status":          session.Status,
		"video_id":        session.VideoID,
	})
}

// CompleteUpload assembles chunks and triggers processing
func CompleteUpload(c *fiber.Ctx) error {
	uploadID := c.Params("uploadId")

	sessionMutex.RLock()
	session, exists := uploadSessions[uploadID]
	sessionMutex.RUnlock()

	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Upload session not found",
		})
	}

	// Verify all chunks are uploaded
	if len(session.UploadedChunks) != session.TotalChunks {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":           "Not all chunks uploaded",
			"uploaded_chunks": len(session.UploadedChunks),
			"total_chunks":    session.TotalChunks,
		})
	}

	// Generate video folder
	folder, videoID := generateFolderWithId()

	// Assemble final file
	finalFilename := filepath.Base(session.Filename)
	finalPath := filepath.Join(folder, finalFilename)

	finalFile, err := os.Create(finalPath)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to create final file",
		})
	}
	defer finalFile.Close()

	// Write chunks in order
	for i := 0; i < session.TotalChunks; i++ {
		chunkPath := filepath.Join(session.TempPath, fmt.Sprintf("chunk_%d", i))
		chunkFile, err := os.Open(chunkPath)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": fmt.Sprintf("Failed to read chunk %d", i),
			})
		}

		_, err = io.Copy(finalFile, chunkFile)
		chunkFile.Close()

		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": fmt.Sprintf("Failed to write chunk %d", i),
			})
		}
	}

	// Update session
	sessionMutex.Lock()
	session.Status = "processing"
	session.VideoID = videoID
	session.FinalPath = finalPath
	session.UpdatedAt = time.Now()
	sessionMutex.Unlock()

	// Clean up temp directory
	go func() {
		os.RemoveAll(session.TempPath)
	}()

	// Create encoding job for tracking
	GetTracker().CreateJob(videoID)

	// Start HLS conversion
	go func() {
		hlsConversionWithResolutions(finalFilename, folder, videoID)

		sessionMutex.Lock()
		session.Status = "complete"
		session.UpdatedAt = time.Now()
		sessionMutex.Unlock()
	}()

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message":  "Upload complete, processing started",
		"video_id": videoID,
		"status":   "processing",
	})
}

// CancelUpload cancels and cleans up an upload session
func CancelUpload(c *fiber.Ctx) error {
	uploadID := c.Params("uploadId")

	sessionMutex.Lock()
	session, exists := uploadSessions[uploadID]
	if exists {
		// Clean up temp files
		os.RemoveAll(session.TempPath)
		delete(uploadSessions, uploadID)
	}
	sessionMutex.Unlock()

	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Upload session not found",
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Upload cancelled",
	})
}
