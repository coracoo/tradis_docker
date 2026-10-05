package api

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestBuildAppListMetaStable(t *testing.T) {
	apps := []App{
		{ID: 1, SortOrder: 1, Name: "one", Version: "1.0", DeploymentCount: 2},
		{ID: 2, SortOrder: 2, Name: "two", Version: "2.0", DeploymentCount: 3},
	}

	first := buildAppListMeta(apps, "cdn-list", time.Unix(100, 0))
	second := buildAppListMeta(apps, "origin-list", time.Unix(200, 0))

	require.NotNil(t, first)
	require.NotNil(t, second)
	require.Equal(t, first.VersionHash, second.VersionHash)
	require.Equal(t, 2, first.TotalCount)
	require.Equal(t, "cdn-list", first.Source)
}

func TestBuildAppListMetaChangesWithList(t *testing.T) {
	apps := []App{{ID: 1, SortOrder: 1, Name: "one", Version: "1.0"}}
	first := buildAppListMeta(apps, "cdn-list", time.Time{})

	apps[0].Version = "1.1"
	second := buildAppListMeta(apps, "cdn-list", time.Time{})

	require.NotEqual(t, first.VersionHash, second.VersionHash)
}

func TestBuildAppListMetaIgnoresDeploymentCount(t *testing.T) {
	apps := []App{{ID: 1, SortOrder: 1, Name: "one", Version: "1.0", DeploymentCount: 1}}
	first := buildAppListMeta(apps, "cdn-list", time.Time{})

	apps[0].DeploymentCount = 99
	second := buildAppListMeta(apps, "cdn-list", time.Time{})

	require.Equal(t, first.VersionHash, second.VersionHash)
}
