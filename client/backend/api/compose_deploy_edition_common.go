package api

import "os"

// composeDeployEditionHooks keeps the common Compose execution path unaware
// of edition-only source preparation. The default remains the manual Compose
// contract used by the local Community build.
type composeDeployEditionHooks struct {
	notifyCompletion  bool
	writeProjectFile  func(string, []byte, os.FileMode) error
	filterDotenv      func(composeRaw, dotenvRaw, projectDir, hostProjectDir string) (string, error)
	prepareProjectDir func(projectDir string, newlyCreated bool) (func() error, error)
}

func defaultComposeDeployEditionHooks() composeDeployEditionHooks {
	return composeDeployEditionHooks{
		notifyCompletion: true,
		writeProjectFile: os.WriteFile,
		filterDotenv: func(composeRaw, dotenvRaw, _ string, _ string) (string, error) {
			return filterDotenvByAllowedKeys(dotenvRaw, extractComposeInterpolationKeys(composeRaw)), nil
		},
		prepareProjectDir: func(string, bool) (func() error, error) {
			return func() error { return nil }, nil
		},
	}
}
