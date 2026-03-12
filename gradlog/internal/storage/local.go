// Package storage provides abstraction for artifact storage backends.
// Currently implements local filesystem storage with support for chunked uploads.
package storage

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Storage defines the interface for artifact storage backends.
type Storage interface {
	// Store writes data to the specified path and returns the SHA-256 checksum.
	Store(path string, data io.Reader) (checksum string, err error)
	// Retrieve returns a reader for the data at the specified path.
	// The caller is responsible for closing the returned reader.
	Retrieve(path string) (io.ReadCloser, error)
	// Delete removes the file at the specified path.
	Delete(path string) error
	// Exists checks if a file exists at the specified path.
	Exists(path string) bool
	// Size returns the size of the file in bytes.
	Size(path string) (int64, error)
}

// LocalStorage implements Storage using the local filesystem.
type LocalStorage struct {
	basePath string
}

// NewLocalStorage creates a new local filesystem storage backend.
// The base path directory will be created if it does not already exist.
func NewLocalStorage(basePath string) (*LocalStorage, error) {
	if err := os.MkdirAll(basePath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create storage directory: %w", err)
	}
	return &LocalStorage{basePath: basePath}, nil
}

// fullPath returns the absolute filesystem path for a relative storage path.
func (s *LocalStorage) fullPath(path string) string {
	return filepath.Join(s.basePath, path)
}

// Store writes data to the specified path, creating parent directories as needed.
// Returns the SHA-256 checksum of the stored data.
func (s *LocalStorage) Store(path string, data io.Reader) (string, error) {
	fullPath := s.fullPath(path)

	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		return "", fmt.Errorf("failed to create directory: %w", err)
	}

	file, err := os.Create(fullPath)
	if err != nil {
		return "", fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	// Write data and compute checksum in a single pass.
	hasher := sha256.New()
	writer := io.MultiWriter(file, hasher)
	if _, err := io.Copy(writer, data); err != nil {
		os.Remove(fullPath) // Clean up on error.
		return "", fmt.Errorf("failed to write file: %w", err)
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}

// Retrieve returns a reader for the file at the specified path.
// The caller is responsible for closing the reader.
func (s *LocalStorage) Retrieve(path string) (io.ReadCloser, error) {
	fullPath := s.fullPath(path)
	file, err := os.Open(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("file not found: %s", path)
		}
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	return file, nil
}

// Delete removes the file at the specified path.
// Returns nil if the file does not exist.
func (s *LocalStorage) Delete(path string) error {
	if err := os.Remove(s.fullPath(path)); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete file: %w", err)
	}
	return nil
}

// Exists reports whether a file exists at the specified path.
func (s *LocalStorage) Exists(path string) bool {
	_, err := os.Stat(s.fullPath(path))
	return err == nil
}

// Size returns the size in bytes of the file at the specified path.
func (s *LocalStorage) Size(path string) (int64, error) {
	info, err := os.Stat(s.fullPath(path))
	if err != nil {
		if os.IsNotExist(err) {
			return 0, fmt.Errorf("file not found: %s", path)
		}
		return 0, fmt.Errorf("failed to stat file: %w", err)
	}
	return info.Size(), nil
}

// StoreChunk writes a single chunk to a temporary storage location.
// Chunks are assembled into the final file via AssembleChunks.
func (s *LocalStorage) StoreChunk(chunkPath string, data io.Reader) (string, error) {
	return s.Store(chunkPath, data)
}

// AssembleChunks concatenates chunk files in order into a single final file.
// Returns the SHA-256 checksum of the assembled file and its total size in bytes.
// Chunk files are deleted after successful assembly.
func (s *LocalStorage) AssembleChunks(chunkPaths []string, finalPath string) (string, int64, error) {
	fullFinalPath := s.fullPath(finalPath)

	if err := os.MkdirAll(filepath.Dir(fullFinalPath), 0755); err != nil {
		return "", 0, fmt.Errorf("failed to create directory: %w", err)
	}

	finalFile, err := os.Create(fullFinalPath)
	if err != nil {
		return "", 0, fmt.Errorf("failed to create final file: %w", err)
	}
	defer finalFile.Close()

	hasher := sha256.New()
	writer := io.MultiWriter(finalFile, hasher)

	var totalSize int64
	for _, chunkPath := range chunkPaths {
		chunkFile, err := s.Retrieve(chunkPath)
		if err != nil {
			os.Remove(fullFinalPath)
			return "", 0, fmt.Errorf("failed to open chunk %s: %w", chunkPath, err)
		}
		n, err := io.Copy(writer, chunkFile)
		chunkFile.Close()
		if err != nil {
			os.Remove(fullFinalPath)
			return "", 0, fmt.Errorf("failed to copy chunk %s: %w", chunkPath, err)
		}
		totalSize += n
	}

	// Remove chunk files now that assembly is complete.
	for _, chunkPath := range chunkPaths {
		s.Delete(chunkPath)
	}

	return hex.EncodeToString(hasher.Sum(nil)), totalSize, nil
}

// RetrieveRange returns a reader for a specific byte range of a stored file.
// Used to serve chunked downloads without loading the entire file into memory.
func (s *LocalStorage) RetrieveRange(path string, start, length int64) (io.ReadCloser, error) {
	fullPath := s.fullPath(path)
	file, err := os.Open(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("file not found: %s", path)
		}
		return nil, fmt.Errorf("failed to open file: %w", err)
	}

	if _, err := file.Seek(start, io.SeekStart); err != nil {
		file.Close()
		return nil, fmt.Errorf("failed to seek to offset %d: %w", start, err)
	}

	return &limitedReadCloser{
		reader: io.LimitReader(file, length),
		closer: file,
	}, nil
}

// limitedReadCloser pairs an io.LimitReader with the underlying io.Closer so
// callers can close the file through the ReadCloser interface.
type limitedReadCloser struct {
	reader io.Reader
	closer io.Closer
}

func (l *limitedReadCloser) Read(p []byte) (int, error) {
	return l.reader.Read(p)
}

func (l *limitedReadCloser) Close() error {
	return l.closer.Close()
}
