// Package integrity provides cryptographic hashing abstractions for file
// integrity verification. It supports streaming reads and pluggable hash
// algorithms, with BLAKE3 as the default.
package integrity

import (
	"encoding/hex"
	"fmt"
	"io"
	"os"

	"github.com/zeebo/blake3"
)

const bufferSize = 32 * 1024 // 32 KB streaming buffer

// Hasher defines the interface for computing cryptographic hashes over
// arbitrary data streams.
type Hasher interface {
	// Hash reads from r and returns the hex-encoded hash digest.
	Hash(reader io.Reader) (string, error)
	// Algorithm returns the name of the hash algorithm (e.g. "blake3").
	Algorithm() string
}

// Blake3Hasher implements the Hasher interface using the BLAKE3 hash function.
type Blake3Hasher struct{}

// NewBlake3Hasher returns a new Blake3Hasher.
func NewBlake3Hasher() *Blake3Hasher {
	return &Blake3Hasher{}
}

// Hash reads all data from reader in 32 KB chunks and returns the hex-encoded
// BLAKE3 digest. It never loads the entire content into memory.
func (b *Blake3Hasher) Hash(reader io.Reader) (string, error) {
	h := blake3.New()
	buf := make([]byte, bufferSize)

	for {
		n, err := reader.Read(buf)
		if n > 0 {
			if _, wErr := h.Write(buf[:n]); wErr != nil {
				return "", fmt.Errorf("blake3 write: %w", wErr)
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("reading data for hash: %w", err)
		}
	}

	sum := h.Sum(nil)
	return hex.EncodeToString(sum), nil
}

// Algorithm returns "blake3".
func (b *Blake3Hasher) Algorithm() string {
	return "blake3"
}

// HashFile computes the BLAKE3 hash of a file on disk using streaming reads.
func (b *Blake3Hasher) HashFile(path string) (string, error) {
	return HashFile(path)
}

// HashFile is a convenience function that opens the file at path and returns
// its hex-encoded BLAKE3 hash. It streams the file in 32 KB chunks.
func HashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("opening file for hash: %w", err)
	}
	defer f.Close()

	hasher := NewBlake3Hasher()
	hash, err := hasher.Hash(f)
	if err != nil {
		return "", fmt.Errorf("hashing file %q: %w", path, err)
	}

	return hash, nil
}
