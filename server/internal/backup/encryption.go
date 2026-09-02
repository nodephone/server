package backup

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"io"
)

// deriveKey derives a 32-byte AES-256 key from a passphrase using SHA-256.
func deriveKey(password string) []byte {
	hash := sha256.Sum256([]byte(password))
	return hash[:]
}

// EncryptData encrypts plaintext payload using AES-256-GCM and a passphrase.
func EncryptData(plainData []byte, password string) ([]byte, error) {
	if password == "" {
		return plainData, nil
	}

	key := deriveKey(password)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create AES cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM cipher: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("failed to generate random nonce: %w", err)
	}

	cipherText := gcm.Seal(nonce, nonce, plainData, nil)
	return cipherText, nil
}

// DecryptData decrypts an AES-256-GCM ciphertext payload using a passphrase.
func DecryptData(cipherData []byte, password string) ([]byte, error) {
	if password == "" {
		return cipherData, nil
	}

	key := deriveKey(password)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create AES cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM cipher: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(cipherData) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short for nonce")
	}

	nonce, cipherText := cipherData[:nonceSize], cipherData[nonceSize:]
	plainText, err := gcm.Open(nil, nonce, cipherText, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt data (invalid password or corrupted payload): %w", err)
	}

	return plainText, nil
}
