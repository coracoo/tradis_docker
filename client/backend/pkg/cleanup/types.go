package cleanup

import (
	"time"

	"github.com/docker/docker/api/types"
)

type Category string

const (
	CategoryBuildCache       Category = "build_cache"
	CategoryDanglingImage    Category = "dangling_image"
	CategoryUnusedImage      Category = "unused_image"
	CategoryStoppedContainer Category = "stopped_container"
	CategoryUnusedNetwork    Category = "unused_network"
	CategoryUnusedVolume     Category = "unused_volume"
)

const BuildCacheAggregateID = "all-unused"

type Risk string

const (
	RiskLow    Risk = "low"
	RiskMedium Risk = "medium"
	RiskHigh   Risk = "high"
)

type SizeState string

const (
	SizeKnown         SizeState = "known"
	SizeUnknown       SizeState = "unknown"
	SizeNotApplicable SizeState = "not_applicable"
)

type Item struct {
	Key               string    `json:"key"`
	Category          Category  `json:"category"`
	ID                string    `json:"id"`
	Name              string    `json:"name"`
	Detail            string    `json:"detail"`
	SizeBytes         int64     `json:"sizeBytes"`
	SizeState         SizeState `json:"sizeState"`
	RelatedContainers []string  `json:"relatedContainers"`
}

type CategoryEvaluation struct {
	Key                   Category `json:"key"`
	Label                 string   `json:"label"`
	Risk                  Risk     `json:"risk"`
	DefaultSelected       bool     `json:"defaultSelected"`
	Count                 int      `json:"count"`
	KnownReclaimableBytes int64    `json:"knownReclaimableBytes"`
	UnknownSizeCount      int      `json:"unknownSizeCount"`
	Items                 []Item   `json:"items"`
}

type Summary struct {
	CandidateCount        int   `json:"candidateCount"`
	KnownReclaimableBytes int64 `json:"knownReclaimableBytes"`
	UnknownSizeCount      int   `json:"unknownSizeCount"`
}

type Evaluation struct {
	EvaluatedAt time.Time            `json:"evaluatedAt"`
	Summary     Summary              `json:"summary"`
	Categories  []CategoryEvaluation `json:"categories"`
}

type Snapshot struct {
	EvaluatedAt             time.Time
	DiskUsage               types.DiskUsage
	Networks                []types.NetworkResource
	ProtectedContainerIDs   map[string]struct{}
	ProtectedContainerNames map[string]struct{}
}

type Selection struct {
	Category Category `json:"category"`
	ID       string   `json:"id"`
}

type Outcome struct {
	Selection
	Name    string `json:"name"`
	Message string `json:"message,omitempty"`
}

type TaskResult struct {
	Deleted                 []Outcome `json:"deleted"`
	Skipped                 []Outcome `json:"skipped"`
	Failed                  []Outcome `json:"failed"`
	SpaceReclaimed          int64     `json:"spaceReclaimed"`
	SpaceReclaimedEstimated bool      `json:"spaceReclaimedEstimated"`
}
