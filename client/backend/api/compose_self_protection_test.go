package api

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSelfProtectionUsesRuntimeComposeProjectInsteadOfFixedName(t *testing.T) {
	require.True(t, isSelfProjectNameForIdentity("nas-panel", "nas-panel", "panel-stack"))
	require.True(t, isSelfProjectNameForIdentity("panel-stack", "nas-panel", "panel-stack"))
	require.False(t, isSelfProjectNameForIdentity("user-project", "nas-panel", "panel-stack"))
}

func TestSelfProtectionKeepsTradisFallbackWhenRuntimeIdentityUnavailable(t *testing.T) {
	require.True(t, isSelfProjectNameForIdentity("tradis", "", ""))
	require.True(t, isSelfProjectNameForIdentity("TRADIS", "", ""))
	require.False(t, isSelfProjectNameForIdentity("user-project", "", ""))
}

func TestContainerIDFromMountInfoContent(t *testing.T) {
	full := "741e8d307cceb5e2e56b802f4fc06706b952db51ceddce4cfae865ebf83a194b"
	other := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	content := "31 23 0:26 / /etc/hostname rw - ext4 /dev/sda1 rw\n" +
		" /var/lib/docker/containers/" + full + "/hostname\n" +
		" /var/lib/docker/containers/" + full + "/hosts\n" +
		" /var/lib/docker/containers/" + other + "/resolv.conf\n"
	require.Equal(t, full, containerIDFromMountInfoContent(content))
	require.Equal(t, "", containerIDFromMountInfoContent("0::/ no containers here"))
	require.Equal(t, "", containerIDFromMountInfoContent("containers/nothex/hostname"))
}
