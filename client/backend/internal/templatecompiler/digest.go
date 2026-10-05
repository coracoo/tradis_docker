package templatecompiler

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
)

func sourceDigest(bundle SourceBundle) (string, error) {
	files := append([]SourceFile(nil), bundle.Files...)
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	payload, err := json.Marshal(struct {
		ComposePath string       `json:"compose_path"`
		Compose     string       `json:"compose"`
		Dotenv      string       `json:"dotenv"`
		Files       []SourceFile `json:"files"`
	}{bundle.ComposePath, bundle.Compose, bundle.Dotenv, files})
	if err != nil {
		return "", err
	}
	return sha256Text(payload), nil
}

func setManifestDigest(manifest *Manifest) error {
	if manifest == nil {
		return fmt.Errorf("manifest is nil")
	}
	manifest.ManifestDigest = ""
	payload, err := json.Marshal(manifest)
	if err != nil {
		return err
	}
	manifest.ManifestDigest = sha256Text(payload)
	return nil
}

func manifestDigest(manifest Manifest) (string, error) {
	manifest.ManifestDigest = ""
	payload, err := json.Marshal(manifest)
	if err != nil {
		return "", err
	}
	return sha256Text(payload), nil
}

func sha256Text(payload []byte) string {
	sum := sha256.Sum256(payload)
	return "sha256:" + hex.EncodeToString(sum[:])
}
