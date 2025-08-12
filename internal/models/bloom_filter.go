package models

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
)

// BloomFilter represents a Bloom Filter for tracking pushed words
type BloomFilter struct {
	UserID    string `json:"userId"`
	BitArray  []byte `json:"bitArray"`  // Serialized bit array
	Size      int    `json:"size"`      // Size of the bit array in bits
	HashCount int    `json:"hashCount"` // Number of hash functions
	UpdatedAt string `json:"updatedAt"` // ISO timestamp
}

// NewBloomFilter creates a new Bloom Filter with specified parameters
func NewBloomFilter(userID string, expectedElements int) *BloomFilter {
	// Use a more reasonable size for our use case
	// For word vocabulary, we don't need huge bit arrays
	size := 8192 // 8KB bit array should be enough for thousands of words
	hashCount := 5 // 5 hash functions is a good balance

	return &BloomFilter{
		UserID:    userID,
		BitArray:  make([]byte, (size+7)/8), // Convert bits to bytes
		Size:      size,
		HashCount: hashCount,
	}
}

// Add adds a word to the Bloom Filter
func (bf *BloomFilter) Add(word string) {
	hashes := bf.getHashes(word)
	for i, hash := range hashes {
		index := hash % uint64(bf.Size)
		byteIndex := index / 8
		bitIndex := index % 8
		
		// Debug: log what we're setting
		if byteIndex < uint64(len(bf.BitArray)) {
			oldByte := bf.BitArray[byteIndex]
			bf.BitArray[byteIndex] |= (1 << bitIndex)
			// Only log if this is a significant word (for debugging)
			if len(word) > 0 && word[0] == 'a' { // Just log words starting with 'a' to reduce noise
				fmt.Printf("Hash %d: word=%s, index=%d, byteIndex=%d, bitIndex=%d, oldByte=%d, newByte=%d\n", 
					i, word, index, byteIndex, bitIndex, oldByte, bf.BitArray[byteIndex])
			}
		}
	}
}

// Contains checks if a word might be in the Bloom Filter
func (bf *BloomFilter) Contains(word string) bool {
	hashes := bf.getHashes(word)
	for _, hash := range hashes {
		index := hash % uint64(bf.Size)
		byteIndex := index / 8
		bitIndex := index % 8
		if bf.BitArray[byteIndex]&(1<<bitIndex) == 0 {
			return false
		}
	}
	return true
}

// getHashes generates multiple hash values for a word
func (bf *BloomFilter) getHashes(word string) []uint64 {
	hashes := make([]uint64, bf.HashCount)
	
	// Use SHA256 as base hash
	hasher := sha256.New()
	hasher.Write([]byte(word))
	hash := hasher.Sum(nil)
	
	// Generate multiple hashes using double hashing technique
	hash1 := binary.BigEndian.Uint64(hash[:8])
	hash2 := binary.BigEndian.Uint64(hash[8:16])
	
	for i := 0; i < bf.HashCount; i++ {
		hashes[i] = hash1 + uint64(i)*hash2
	}
	
	return hashes
}

// calculateOptimalSize calculates optimal bit array size
func calculateOptimalSize(expectedElements int, _ float64) int {
	// m = -(n * ln(p)) / (ln(2))^2
	// where n = expected elements, p = false positive rate
	// Simplified calculation for practical use
	return expectedElements * 10 // Simple approximation
}

// calculateOptimalHashCount calculates optimal number of hash functions
func calculateOptimalHashCount(size, expectedElements int) int {
	// k = (m/n) * ln(2)
	// where m = size, n = expected elements
	// Simplified to a practical range
	hashCount := (size / expectedElements)
	if hashCount < 3 {
		return 3
	}
	if hashCount > 7 {
		return 7
	}
	return hashCount
}