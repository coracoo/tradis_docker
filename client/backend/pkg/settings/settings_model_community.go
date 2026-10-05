//go:build community

package settings

// Settings contains only locally managed, community-edition configuration.
// Commercial configuration is intentionally absent from both this type and
// its JSON representation.
type Settings struct {
	LanUrl                        string   `json:"lanUrl"`
	WanUrl                        string   `json:"wanUrl"`
	AppStoreCDNURL                string   `json:"appStoreCDNURL"`
	TutorialRSSURL                string   `json:"tutorialRSSURL"`
	AdvancedMode                  bool     `json:"advancedMode"`
	AllocPortStart                int      `json:"allocPortStart"`
	AllocPortEnd                  int      `json:"allocPortEnd"`
	AllowAutoAllocPort            bool     `json:"allowAutoAllocPort"`
	ImageUpdateIntervalMinutes    int      `json:"imageUpdateIntervalMinutes"`
	AiEnabled                     bool     `json:"aiEnabled"`
	AiBaseUrl                     string   `json:"aiBaseUrl"`
	AiApiKey                      string   `json:"aiApiKey,omitempty"`
	AiApiKeySet                   bool     `json:"aiApiKeySet"`
	AiApiKeyStored                bool     `json:"aiApiKeyStored"`
	AiApiKeySource                string   `json:"aiApiKeySource"`
	AiModel                       string   `json:"aiModel"`
	AiUtilityModel                string   `json:"aiUtilityModel"`
	AiTemperature                 float64  `json:"aiTemperature"`
	AiMaxTokens                   int      `json:"aiMaxTokens"`
	AiAllowCreateCategory         bool     `json:"aiAllowCreateCategory"`
	AiNavigationPrompt            string   `json:"aiNavigationPrompt"`
	AiComposePrompt               string   `json:"aiComposePrompt"`
	VolumeBackupEnabled           bool     `json:"volumeBackupEnabled"`
	VolumeBackupImage             string   `json:"volumeBackupImage"`
	VolumeBackupEnv               string   `json:"volumeBackupEnv,omitempty"`
	VolumeBackupEnvSet            bool     `json:"volumeBackupEnvSet"`
	VolumeBackupEnvStored         bool     `json:"volumeBackupEnvStored"`
	VolumeBackupCronExpression    string   `json:"volumeBackupCronExpression"`
	VolumeBackupVolumes           []string `json:"volumeBackupVolumes"`
	VolumeBackupArchiveDir        string   `json:"volumeBackupArchiveDir"`
	VolumeBackupMountDockerSock   bool     `json:"volumeBackupMountDockerSock"`
	NotificationEnabledCategories []string `json:"notificationEnabledCategories"`
}
