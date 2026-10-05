package api

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestComposeExtraFilesValidateBeforeWriting(t *testing.T) {
	for _, conflict := range []string{"directory", "parent_file", "core_file", "alias", "core_parent_alias"} {
		t.Run(conflict, func(t *testing.T) {
			dir := t.TempDir()
			original := filepath.Join(dir, "a.conf")
			require.NoError(t, os.WriteFile(original, []byte("original"), 0600))
			files := map[string]string{"a.conf": "changed"}
			switch conflict {
			case "directory":
				require.NoError(t, os.Mkdir(filepath.Join(dir, "z.conf"), 0755))
				files["z.conf"] = "server config"
			case "parent_file":
				require.NoError(t, os.WriteFile(filepath.Join(dir, "z"), []byte("keep"), 0600))
				files["z/config.conf"] = "server config"
			case "core_file":
				files["compose.yml"] = "services: {}"
			case "alias":
				files["./a.conf"] = "other value"
			case "core_parent_alias":
				require.NoError(t, os.WriteFile(filepath.Join(dir, "compose.yml"), []byte("services: {}"), 0600))
				require.NoError(t, os.Symlink(dir, filepath.Join(dir, "alias")))
				files["alias/compose.yml"] = "changed core"
			}
			_, err := writeComposeExtraFiles(dir, files)
			require.Error(t, err)
			data, err := os.ReadFile(original)
			require.NoError(t, err)
			require.Equal(t, "original", string(data), "a rejected batch must not overwrite earlier files")
		})
	}
}

func TestComposeExtraFilesPreserveModeAndDirectoryConflict(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "entrypoint.sh")
	require.NoError(t, os.WriteFile(path, []byte("old"), 0750))
	written, err := writeComposeExtraFiles(dir, map[string]string{"entrypoint.sh": "new", "config/Caddyfile": "respond ok"})
	require.NoError(t, err)
	require.Equal(t, []string{"config/Caddyfile", "entrypoint.sh"}, written)
	info, err := os.Stat(path)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0750), info.Mode().Perm())
	require.NoError(t, os.Mkdir(filepath.Join(dir, "Caddyfile"), 0755))
	_, err = writeComposeExtraFiles(dir, map[string]string{"Caddyfile": "respond ok"})
	require.ErrorContains(t, err, "config_file_type_conflict")
	info, err = os.Stat(filepath.Join(dir, "Caddyfile"))
	require.NoError(t, err)
	require.True(t, info.IsDir(), "never remove a directory to make room for a config file")
}
