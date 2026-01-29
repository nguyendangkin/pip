package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"io"

	"golang.org/x/crypto/hkdf"
	"golang.org/x/crypto/pbkdf2"
)

const (
	KeySize   = 32
	NonceSize = 12
	SaltSize  = 16 // 16 bytes salt
	Iter      = 100_000
)

var (
	ErrInvalidPassword = errors.New("invalid password")
)

// DeriveKey derives a 32-byte Master Key from password and salt using PBKDF2.
// This is CPU intensive to resist brute-force.
func DeriveKey(password []byte, salt []byte) []byte {
	return pbkdf2.Key(password, salt, Iter, KeySize, sha256.New)
}

// DeriveStreamKey derives a unique 32-byte File Key from the Master Key and a File Salt using HKDF.
// This is fast and cryptographic safe for per-file key generation.
func DeriveStreamKey(masterKey []byte, fileSalt []byte) ([]byte, error) {
	hkdf := hkdf.New(sha256.New, masterKey, fileSalt, []byte("chin-stream-v6"))
	key := make([]byte, KeySize)
	if _, err := io.ReadFull(hkdf, key); err != nil {
		return nil, err
	}
	return key, nil
}

func GenerateSalt() ([]byte, error) {
	salt := make([]byte, SaltSize)
	_, err := io.ReadFull(rand.Reader, salt)
	if err != nil {
		return nil, err
	}
	return salt, nil
}

// Encrypt encrypts data in-memory (for Metadata).
// It uses the standard DeriveKey (PBKDF2) directly.
func Encrypt(data []byte, password []byte, salt []byte) ([]byte, []byte, error) {
	key := DeriveKey(password, salt)

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, err
	}

	nonce := make([]byte, NonceSize)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, nil, err
	}

	ciphertext := gcm.Seal(nil, nonce, data, nil)

	return ciphertext, nonce, nil
}

func Decrypt(ciphertext []byte, nonce []byte, password []byte, salt []byte) ([]byte, error) {
	key := DeriveKey(password, salt)

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, ErrInvalidPassword
	}

	return plaintext, nil
}
