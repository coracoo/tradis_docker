package secrets

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const (
	masterKeyEnv                = "TRADIS_MASTER_KEY"
	masterKeyFileEnv            = "TRADIS_MASTER_KEY_FILE"
	previousMasterKeysEnv       = "TRADIS_PREVIOUS_MASTER_KEYS"
	masterKeySize               = 32
	ciphertextPrefix            = "tradis:v1:"
	persistentMasterKeyFilename = ".tradis-master-key"
)

var (
	ErrMasterKeyUnavailable = errors.New("TRADIS master key is not configured")
	ErrCannotDecrypt        = errors.New("cannot decrypt sensitive value")
	persistentMasterKeyMu   sync.RWMutex
	persistentMasterKeyPath string
)

// ConfigurePersistentMasterKey ensures a default installation key exists when
// the operator has not supplied TRADIS_MASTER_KEY or TRADIS_MASTER_KEY_FILE.
// The generated file lives with the persistent application data so container
// recreation does not invalidate encrypted settings.
func ConfigurePersistentMasterKey(dataDir string) error {
	if raw := strings.TrimSpace(os.Getenv(masterKeyEnv)); raw != "" {
		if _, err := decodeMasterKey(raw); err != nil {
			return err
		}
		return registerExistingPersistentMasterKey(dataDir)
	}
	if configuredPath := strings.TrimSpace(os.Getenv(masterKeyFileEnv)); configuredPath != "" {
		raw, err := os.ReadFile(configuredPath)
		if err != nil {
			return fmt.Errorf("read configured TRADIS master key file: %w", err)
		}
		if _, err := decodeMasterKey(strings.TrimSpace(string(raw))); err != nil {
			return err
		}
		return registerExistingPersistentMasterKey(dataDir)
	}

	dataDir = strings.TrimSpace(dataDir)
	if dataDir == "" {
		return ErrMasterKeyUnavailable
	}
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return fmt.Errorf("create TRADIS data directory: %w", err)
	}

	keyPath := filepath.Join(dataDir, persistentMasterKeyFilename)
	raw, err := readOrCreatePersistentMasterKey(keyPath)
	if err != nil {
		return err
	}
	if _, err := decodeMasterKey(raw); err != nil {
		return fmt.Errorf("invalid persistent TRADIS master key: %w", err)
	}

	persistentMasterKeyMu.Lock()
	persistentMasterKeyPath = keyPath
	persistentMasterKeyMu.Unlock()
	return nil
}

func registerExistingPersistentMasterKey(dataDir string) error {
	dataDir = strings.TrimSpace(dataDir)
	if dataDir == "" {
		return nil
	}
	keyPath := filepath.Join(dataDir, persistentMasterKeyFilename)
	raw, err := os.ReadFile(keyPath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read existing persistent TRADIS master key: %w", err)
	}
	if _, err := decodeMasterKey(strings.TrimSpace(string(raw))); err != nil {
		return fmt.Errorf("invalid existing persistent TRADIS master key: %w", err)
	}
	if err := os.Chmod(keyPath, 0600); err != nil {
		return fmt.Errorf("secure existing persistent TRADIS master key: %w", err)
	}
	persistentMasterKeyMu.Lock()
	persistentMasterKeyPath = keyPath
	persistentMasterKeyMu.Unlock()
	return nil
}

func readOrCreatePersistentMasterKey(path string) (string, error) {
	raw, err := os.ReadFile(path)
	if err == nil {
		if chmodErr := os.Chmod(path, 0600); chmodErr != nil {
			return "", fmt.Errorf("secure persistent TRADIS master key: %w", chmodErr)
		}
		return strings.TrimSpace(string(raw)), nil
	}
	if !os.IsNotExist(err) {
		return "", fmt.Errorf("read persistent TRADIS master key: %w", err)
	}

	key := make([]byte, masterKeySize)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return "", fmt.Errorf("generate persistent TRADIS master key: %w", err)
	}
	encoded := base64.StdEncoding.EncodeToString(key)
	err = writePersistentMasterKey(path, encoded)
	if os.IsExist(err) {
		return readOrCreatePersistentMasterKey(path)
	}
	if err != nil {
		return "", fmt.Errorf("create persistent TRADIS master key: %w", err)
	}
	return encoded, nil
}

func writePersistentMasterKey(path, encoded string) error {
	directory := filepath.Dir(path)
	file, err := os.CreateTemp(directory, "."+filepath.Base(path)+"-*")
	if err != nil {
		return err
	}
	tempPath := file.Name()
	defer func() {
		_ = file.Close()
		_ = os.Remove(tempPath)
	}()
	if err := file.Chmod(0600); err != nil {
		return err
	}
	if _, err := file.WriteString(encoded + "\n"); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.Link(tempPath, path); err != nil {
		return err
	}
	if directoryHandle, err := os.Open(directory); err == nil {
		defer directoryHandle.Close()
		if err := directoryHandle.Sync(); err != nil {
			return err
		}
	}
	return nil
}

// Available reports whether the active process has a valid primary master key.
func Available() bool {
	_, err := primaryMasterKey()
	return err == nil
}

// IsSealed reports whether value uses TRADIS's versioned encrypted envelope.
func IsSealed(value string) bool {
	return strings.HasPrefix(strings.TrimSpace(value), ciphertextPrefix)
}

// DeriveKey returns a purpose-bound process key without exposing the primary
// installation key to callers. It is stable across restarts while the primary
// key remains available.
func DeriveKey(purpose string) ([]byte, error) {
	purpose = strings.TrimSpace(purpose)
	if purpose == "" {
		return nil, fmt.Errorf("key derivation purpose is required")
	}
	key, err := primaryMasterKey()
	if err != nil {
		return nil, err
	}
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte("tradis-derived-key:" + purpose))
	return mac.Sum(nil), nil
}

// Seal encrypts plaintext using AES-256-GCM and authenticates the caller context.
func Seal(plaintext, context string) (string, error) {
	key, err := primaryMasterKey()
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("create AES cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("create GCM cipher: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("generate encryption nonce: %w", err)
	}
	payload := append(nonce, gcm.Seal(nil, nonce, []byte(plaintext), associatedData(context))...)
	return ciphertextPrefix + base64.RawURLEncoding.EncodeToString(payload), nil
}

// Open decrypts a sealed value. The primary key is tried first, followed by
// configured previous keys so a controlled master-key rotation remains readable.
func Open(ciphertext, context string) (string, error) {
	encoded := strings.TrimPrefix(strings.TrimSpace(ciphertext), ciphertextPrefix)
	if encoded == strings.TrimSpace(ciphertext) {
		return "", ErrCannotDecrypt
	}
	payload, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return "", ErrCannotDecrypt
	}

	keys, err := configuredMasterKeys()
	if err != nil {
		return "", err
	}
	for _, key := range keys {
		plaintext, openErr := openWithKey(key, payload, context)
		if openErr == nil {
			return string(plaintext), nil
		}
	}
	return "", ErrCannotDecrypt
}

func associatedData(context string) []byte {
	return []byte("tradis:" + strings.TrimSpace(context))
}

func openWithKey(key []byte, payload []byte, context string) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(payload) < gcm.NonceSize()+gcm.Overhead() {
		return nil, ErrCannotDecrypt
	}
	return gcm.Open(nil, payload[:gcm.NonceSize()], payload[gcm.NonceSize():], associatedData(context))
}

func primaryMasterKey() ([]byte, error) {
	raw := strings.TrimSpace(os.Getenv(masterKeyEnv))
	if raw == "" {
		path := strings.TrimSpace(os.Getenv(masterKeyFileEnv))
		if path == "" {
			persistentMasterKeyMu.RLock()
			path = persistentMasterKeyPath
			persistentMasterKeyMu.RUnlock()
		}
		if path != "" {
			data, err := os.ReadFile(path)
			if err != nil {
				return nil, ErrMasterKeyUnavailable
			}
			raw = strings.TrimSpace(string(data))
		}
	}
	return decodeMasterKey(raw)
}

func configuredMasterKeys() ([][]byte, error) {
	primary, err := primaryMasterKey()
	if err != nil {
		return nil, err
	}
	keys := make([][]byte, 0, 3)
	appendUnique := func(key []byte) {
		for _, existing := range keys {
			if bytes.Equal(existing, key) {
				return
			}
		}
		keys = append(keys, key)
	}
	appendUnique(primary)
	for _, raw := range strings.Split(os.Getenv(previousMasterKeysEnv), ",") {
		key, decodeErr := decodeMasterKey(strings.TrimSpace(raw))
		if decodeErr == nil {
			appendUnique(key)
		}
	}
	persistentMasterKeyMu.RLock()
	fallbackPath := persistentMasterKeyPath
	persistentMasterKeyMu.RUnlock()
	if fallbackPath != "" {
		if raw, readErr := os.ReadFile(fallbackPath); readErr == nil {
			if key, decodeErr := decodeMasterKey(strings.TrimSpace(string(raw))); decodeErr == nil {
				appendUnique(key)
			}
		}
	}
	return keys, nil
}

func decodeMasterKey(raw string) ([]byte, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return nil, ErrMasterKeyUnavailable
	}
	for _, decode := range []func(string) ([]byte, error){
		hex.DecodeString,
		base64.StdEncoding.DecodeString,
		base64.RawStdEncoding.DecodeString,
	} {
		decoded, err := decode(value)
		if err == nil && len(decoded) == masterKeySize {
			return decoded, nil
		}
	}
	return nil, ErrMasterKeyUnavailable
}
