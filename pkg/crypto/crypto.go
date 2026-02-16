package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"io"
)

func DeriveKey(secret string) [32]byte {
	return sha256.Sum256([]byte(secret))
}

// NewCipher creates a generic block cipher from the secret.
// We use SHA-256 to hash the secret into a 32-byte key for AES-256.
func NewGCM(secret string) (cipher.AEAD, error) {
	key := DeriveKey(secret)
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	return gcm, nil
}

// Encrypt encrypts plaintext using the given secret.
// It prepends the nonce to the ciphertext.
func Encrypt(secret string, plaintext []byte) ([]byte, error) {
	gcm, err := NewGCM(secret)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return ciphertext, nil
}

// Decrypt decrypts ciphertext using the given secret.
// It expects the nonce to be prepended to the ciphertext.
func Decrypt(secret string, ciphertext []byte) ([]byte, error) {
	gcm, err := NewGCM(secret)
	if err != nil {
		return nil, err
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}

	nonce, actualCiphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, actualCiphertext, nil)
	if err != nil {
		return nil, err
	}

	return plaintext, nil
}

// StreamEncrypter wraps an io.Writer to encrypt data on the fly.
// NOTE: For a high-performance HTTP tunnel where we might send chunks,
// standard block encryption per chunk is basic.
// Because we are tunneling strictly over HTTP bodies which might be chunked,
// and we want to look like file upload/download, we probably want to just stream.
// However, implementing a proper streaming crypto reader/writer (like using OFB or CTR)
// is better than independent block encryption if we want a continuous stream.
// BUT, if we are doing HTTP chunks, we might just encrypt each "packet" payload.
// For simplicity and robustness with standard connection behavior, let's use
// a stream cipher approach (AES-CTR) or stick to chunk-based if we frame data.
// Given strict "high performance" and "standard http api", generic stream wrapper is best.
// Let's implement a wrapper that negotiates a stream.

type CryptoWriter struct {
	w      io.Writer
	stream cipher.Stream
}

func NewCryptoWriter(w io.Writer, secret string, iv []byte) (*CryptoWriter, error) {
	key := DeriveKey(secret)
	return NewCryptoWriterWithKey(w, key, iv)
}

func NewCryptoWriterWithKey(w io.Writer, key [32]byte, iv []byte) (*CryptoWriter, error) {
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}

	// For CTR, IV size = block size
	if len(iv) != aes.BlockSize {
		return nil, fmt.Errorf("iv length must be %d", aes.BlockSize)
	}

	stream := cipher.NewCTR(block, iv)
	return &CryptoWriter{w: w, stream: stream}, nil
}

func (cw *CryptoWriter) Write(p []byte) (n int, err error) {
	out := make([]byte, len(p))
	cw.stream.XORKeyStream(out, p)
	return cw.w.Write(out)
}

type CryptoReader struct {
	r      io.Reader
	stream cipher.Stream
}

func NewCryptoReader(r io.Reader, secret string, iv []byte) (*CryptoReader, error) {
	key := DeriveKey(secret)
	return NewCryptoReaderWithKey(r, key, iv)
}

func NewCryptoReaderWithKey(r io.Reader, key [32]byte, iv []byte) (*CryptoReader, error) {
	// CTR mode is symmetric for encryption/decryption
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}

	if len(iv) != aes.BlockSize {
		return nil, fmt.Errorf("iv length must be %d", aes.BlockSize)
	}

	stream := cipher.NewCTR(block, iv)
	return &CryptoReader{r: r, stream: stream}, nil
}

func (cr *CryptoReader) Read(p []byte) (n int, err error) {
	n, err = cr.r.Read(p)
	if n > 0 {
		cr.stream.XORKeyStream(p[:n], p[:n])
	}
	return n, err
}

func XORCTRInPlace(key [32]byte, iv []byte, data []byte) error {
	if len(iv) != aes.BlockSize {
		return fmt.Errorf("iv length must be %d", aes.BlockSize)
	}
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return err
	}
	cipher.NewCTR(block, iv).XORKeyStream(data, data)
	return nil
}
