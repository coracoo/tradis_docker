//go:build community

package api

import "strings"

func setEditionAppStoreSource(app *App) {
	if app == nil {
		return
	}
	app.ShowDeployCount = false
	app.DeploymentCount = 0
}

func applyEditionAppStoreCounts(apps []App) {
	for i := range apps {
		setEditionAppStoreSource(&apps[i])
	}
}

func applyEditionAppStoreCount(app *App) {
	setEditionAppStoreSource(app)
}

// Preserve the cache accounting semantics while ignoring a legacy Full-edition file.
func isEditionAppStoreCacheFile(name string) bool {
	return strings.HasSuffix(name, "deployment-counts.json")
}
