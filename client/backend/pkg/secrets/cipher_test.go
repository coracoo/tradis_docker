package secrets

import (
	"bytes"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testMasterKey(fill byte) string {
	return base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{fill}, masterKeySize))
}

func TestSealOpenBindsCiphertextToContext(t *testing.T) {
	t.Setenv(masterKeyEnv, testMasterKey(1))
	t.Setenv(previousMasterKeysEnv, "")

	sealed, err := Seal("correct horse battery staple", "settings:ai_api_key")
	if err != nil {
		t.Fatalf("Seal() error = %v", err)
	}
	if !IsSealed(sealed) {
		t.Fatalf("Seal() returned an unrecognised value: %q", sealed)
	}
	if strings.Contains(sealed, "correct horse") {
		t.Fatalf("Seal() leaked plaintext: %q", sealed)
	}

	opened, err := Open(sealed, "settings:ai_api_key")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if opened != "correct horse battery staple" {
		t.Fatalf("Open() = %q", opened)
	}

	if _, err := Open(sealed, "settings:github_token"); !errors.Is(err, ErrCannotDecrypt) {
		t.Fatalf("Open() with a different context error = %v, want ErrCannotDecrypt", err)
	}
}

func TestSealUsesFreshNonceForSamePlaintext(t *testing.T) {
	t.Setenv(masterKeyEnv, testMasterKey(2))
	t.Setenv(previousMasterKeysEnv, "")

	first, err := Seal("same-value", "settings:test")
	if err != nil {
		t.Fatalf("Seal() first error = %v", err)
	}
	second, err := Seal("same-value", "settings:test")
	if err != nil {
		t.Fatalf("Seal() second error = %v", err)
	}
	if first == second {
		t.Fatal("Seal() reused a nonce for identical plaintext")
	}
}

func TestOpenRejectsTamperedCiphertext(t *testing.T) {
	t.Setenv(masterKeyEnv, testMasterKey(3))
	t.Setenv(previousMasterKeysEnv, "")

	sealed, err := Seal("sensitive", "settings:test")
	if err != nil {
		t.Fatalf("Seal() error = %v", err)
	}
	payload, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(sealed, ciphertextPrefix))
	if err != nil {
		t.Fatalf("decode sealed payload: %v", err)
	}
	payload[len(payload)-1] ^= 0x01
	tampered := ciphertextPrefix + base64.RawURLEncoding.EncodeToString(payload)

	if _, err := Open(tampered, "settings:test"); !errors.Is(err, ErrCannotDecrypt) {
		t.Fatalf("Open(tampered) error = %v, want ErrCannotDecrypt", err)
	}
}

func TestOpenSupportsPreviousMasterKeyDuringRotation(t *testing.T) {
	oldKey := testMasterKey(4)
	t.Setenv(masterKeyEnv, oldKey)
	t.Setenv(previousMasterKeysEnv, "")

	sealed, err := Seal("rotatable", "settings:test")
	if err != nil {
		t.Fatalf("Seal() error = %v", err)
	}

	t.Setenv(masterKeyEnv, testMasterKey(5))
	t.Setenv(previousMasterKeysEnv, oldKey)
	opened, err := Open(sealed, "settings:test")
	if err != nil {
		t.Fatalf("Open() with previous key error = %v", err)
	}
	if opened != "rotatable" {
		t.Fatalf("Open() = %q", opened)
	}
}

func TestSealFailsWithoutMasterKey(t *testing.T) {
	t.Setenv(masterKeyEnv, "")
	t.Setenv(masterKeyFileEnv, "")
	t.Setenv(previousMasterKeysEnv, "")

	if Available() {
		t.Fatal("Available() = true without a configured master key")
	}
	if _, err := Seal("sensitive", "settings:test"); !errors.Is(err, ErrMasterKeyUnavailable) {
		t.Fatalf("Seal() error = %v, want ErrMasterKeyUnavailable", err)
	}
}

func TestDeriveKeyIsStableAndPurposeBound(t *testing.T) {
	t.Setenv(masterKeyEnv, testMasterKey(11))
	t.Setenv(masterKeyFileEnv, "")
	t.Setenv(previousMasterKeysEnv, "")
	first, err := DeriveKey("ai-agent-approval:v1")
	if err != nil {
		t.Fatal(err)
	}
	second, err := DeriveKey("ai-agent-approval:v1")
	if err != nil {
		t.Fatal(err)
	}
	other, err := DeriveKey("another-purpose")
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 32 || !bytes.Equal(first, second) {
		t.Fatalf("derived key is not stable: %x %x", first, second)
	}
	if bytes.Equal(first, other) {
		t.Fatal("different purposes derived the same key")
	}
}

func TestSealCanReadMasterKeyFromReadOnlyFile(t *testing.T) {
	keyPath := filepath.Join(t.TempDir(), "master-key")
	if err := os.WriteFile(keyPath, []byte(testMasterKey(6)+"\n"), 0400); err != nil {
		t.Fatalf("write key file: %v", err)
	}
	t.Setenv(masterKeyEnv, "")
	t.Setenv(masterKeyFileEnv, keyPath)
	t.Setenv(previousMasterKeysEnv, "")

	sealed, err := Seal("from-file", "settings:test")
	if err != nil {
		t.Fatalf("Seal() error = %v", err)
	}
	opened, err := Open(sealed, "settings:test")
	if err != nil || opened != "from-file" {
		t.Fatalf("Open() = %q, %v", opened, err)
	}
}

func TestConfigurePersistentMasterKeyGeneratesAndReusesInstallationKey(t *testing.T) {
	t.Setenv(masterKeyEnv, "")
	t.Setenv(masterKeyFileEnv, "")
	t.Setenv(previousMasterKeysEnv, "")
	resetPersistentMasterKeyForTest(t)

	dataDir := t.TempDir()
	if err := ConfigurePersistentMasterKey(dataDir); err != nil {
		t.Fatalf("ConfigurePersistentMasterKey() error = %v", err)
	}
	if !Available() {
		t.Fatal("Available() = false after persistent key initialization")
	}

	keyPath := filepath.Join(dataDir, persistentMasterKeyFilename)
	firstKey, err := os.ReadFile(keyPath)
	if err != nil {
		t.Fatalf("read generated master key: %v", err)
	}
	info, err := os.Stat(keyPath)
	if err != nil {
		t.Fatalf("stat generated master key: %v", err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("generated master key mode = %o, want 600", info.Mode().Perm())
	}

	sealed, err := Seal("persisted-secret", "settings:test")
	if err != nil {
		t.Fatalf("Seal() error = %v", err)
	}

	resetPersistentMasterKeyForTest(t)
	if err := ConfigurePersistentMasterKey(dataDir); err != nil {
		t.Fatalf("ConfigurePersistentMasterKey() reuse error = %v", err)
	}
	secondKey, err := os.ReadFile(keyPath)
	if err != nil {
		t.Fatalf("read reused master key: %v", err)
	}
	if !bytes.Equal(firstKey, secondKey) {
		t.Fatal("ConfigurePersistentMasterKey() replaced the installation key")
	}
	opened, err := Open(sealed, "settings:test")
	if err != nil || opened != "persisted-secret" {
		t.Fatalf("Open() after restart = %q, %v", opened, err)
	}
}

func TestConfigurePersistentMasterKeyKeepsExplicitConfigurationAuthoritative(t *testing.T) {
	t.Setenv(masterKeyEnv, testMasterKey(7))
	t.Setenv(masterKeyFileEnv, "")
	resetPersistentMasterKeyForTest(t)

	dataDir := t.TempDir()
	if err := ConfigurePersistentMasterKey(dataDir); err != nil {
		t.Fatalf("ConfigurePersistentMasterKey() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(dataDir, persistentMasterKeyFilename)); !os.IsNotExist(err) {
		t.Fatalf("persistent key file exists with explicit configuration: %v", err)
	}
}

func TestConfigurePersistentMasterKeyKeepsExistingInstallationKeyAsRotationFallback(t *testing.T) {
	t.Setenv(masterKeyEnv, "")
	t.Setenv(masterKeyFileEnv, "")
	t.Setenv(previousMasterKeysEnv, "")
	resetPersistentMasterKeyForTest(t)

	dataDir := t.TempDir()
	if err := ConfigurePersistentMasterKey(dataDir); err != nil {
		t.Fatalf("initial ConfigurePersistentMasterKey() error = %v", err)
	}
	sealed, err := Seal("existing-ai-key", "settings:ai_api_key")
	if err != nil {
		t.Fatalf("Seal() with installation key error = %v", err)
	}

	t.Setenv(masterKeyEnv, testMasterKey(8))
	if err := ConfigurePersistentMasterKey(dataDir); err != nil {
		t.Fatalf("rotated ConfigurePersistentMasterKey() error = %v", err)
	}
	opened, err := Open(sealed, "settings:ai_api_key")
	if err != nil || opened != "existing-ai-key" {
		t.Fatalf("Open() with installation fallback = %q, %v", opened, err)
	}
}

func TestWritePersistentMasterKeyDoesNotReplaceExistingKey(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, persistentMasterKeyFilename)
	if err := os.WriteFile(path, []byte("existing\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := writePersistentMasterKey(path, "replacement"); !os.IsExist(err) {
		t.Fatalf("writePersistentMasterKey error = %v, want os.ErrExist", err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "existing\n" {
		t.Fatalf("existing key was replaced: %q", content)
	}
	matches, err := filepath.Glob(filepath.Join(directory, "."+persistentMasterKeyFilename+"-*"))
	if err != nil || len(matches) != 0 {
		t.Fatalf("temporary key files remain: %#v, %v", matches, err)
	}
}

func resetPersistentMasterKeyForTest(t *testing.T) {
	t.Helper()
	persistentMasterKeyMu.Lock()
	previous := persistentMasterKeyPath
	persistentMasterKeyPath = ""
	persistentMasterKeyMu.Unlock()
	t.Cleanup(func() {
		persistentMasterKeyMu.Lock()
		persistentMasterKeyPath = previous
		persistentMasterKeyMu.Unlock()
	})
}
