//go:build community

package api

func composeDeployEditionHooksForTask(string) composeDeployEditionHooks {
	return defaultComposeDeployEditionHooks()
}

func shouldHideEditionComposeDraft(string, bool) bool { return false }
