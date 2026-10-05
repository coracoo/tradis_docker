package cleanup

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
)

type categoryDefinition struct {
	key             Category
	label           string
	risk            Risk
	defaultSelected bool
}

var categoryDefinitions = []categoryDefinition{
	{key: CategoryBuildCache, label: "构建缓存", risk: RiskLow, defaultSelected: true},
	{key: CategoryDanglingImage, label: "悬空镜像", risk: RiskLow, defaultSelected: true},
	{key: CategoryUnusedImage, label: "未使用镜像", risk: RiskMedium},
	{key: CategoryStoppedContainer, label: "已停止容器", risk: RiskMedium},
	{key: CategoryUnusedNetwork, label: "未使用网络", risk: RiskLow, defaultSelected: true},
	{key: CategoryUnusedVolume, label: "未使用数据卷", risk: RiskHigh},
}

func Evaluate(snapshot Snapshot) Evaluation {
	itemsByCategory := make(map[Category][]Item, len(categoryDefinitions))
	referencedImages := make(map[string]struct{})
	referencedVolumes := make(map[string]struct{})

	for _, container := range snapshot.DiskUsage.Containers {
		if container == nil {
			continue
		}
		if imageID := strings.TrimSpace(container.ImageID); imageID != "" {
			referencedImages[imageID] = struct{}{}
		}
		for _, mount := range container.Mounts {
			if name := strings.TrimSpace(mount.Name); name != "" {
				referencedVolumes[name] = struct{}{}
			}
		}

		if !isStoppedContainerState(container.State) || isProtectedContainer(snapshot, *container) {
			continue
		}
		itemsByCategory[CategoryStoppedContainer] = append(itemsByCategory[CategoryStoppedContainer], Item{
			Key:       itemKey(CategoryStoppedContainer, container.ID),
			Category:  CategoryStoppedContainer,
			ID:        container.ID,
			Name:      firstContainerName(*container),
			Detail:    strings.TrimSpace(container.Image),
			SizeBytes: maxInt64(container.SizeRw, 0),
			SizeState: SizeKnown,
		})
	}

	for _, image := range snapshot.DiskUsage.Images {
		if image == nil {
			continue
		}
		id := strings.TrimSpace(image.ID)
		if id == "" {
			continue
		}
		if _, used := referencedImages[id]; used {
			continue
		}
		category := CategoryUnusedImage
		if isDanglingImage(*image) {
			category = CategoryDanglingImage
		}
		itemsByCategory[category] = append(itemsByCategory[category], Item{
			Key:       itemKey(category, id),
			Category:  category,
			ID:        id,
			Name:      firstImageName(*image),
			Detail:    shortID(id),
			SizeBytes: maxInt64(image.Size, 0),
			SizeState: SizeKnown,
		})
	}

	appendBuildCacheCandidate(itemsByCategory, snapshot.DiskUsage.BuildCache)

	for _, network := range snapshot.Networks {
		if !isUnusedNetwork(network) {
			continue
		}
		itemsByCategory[CategoryUnusedNetwork] = append(itemsByCategory[CategoryUnusedNetwork], Item{
			Key:       itemKey(CategoryUnusedNetwork, network.ID),
			Category:  CategoryUnusedNetwork,
			ID:        network.ID,
			Name:      network.Name,
			Detail:    network.Driver,
			SizeState: SizeNotApplicable,
		})
	}

	for _, item := range snapshot.DiskUsage.Volumes {
		if item == nil {
			continue
		}
		name := strings.TrimSpace(item.Name)
		if name == "" {
			continue
		}
		if _, used := referencedVolumes[name]; used {
			continue
		}
		if item.UsageData != nil && item.UsageData.RefCount > 0 {
			continue
		}
		sizeState := SizeUnknown
		sizeBytes := int64(0)
		if item.UsageData != nil && item.UsageData.Size >= 0 {
			sizeState = SizeKnown
			sizeBytes = item.UsageData.Size
		}
		itemsByCategory[CategoryUnusedVolume] = append(itemsByCategory[CategoryUnusedVolume], Item{
			Key:       itemKey(CategoryUnusedVolume, name),
			Category:  CategoryUnusedVolume,
			ID:        name,
			Name:      name,
			Detail:    firstNonEmpty(item.Driver, item.Scope),
			SizeBytes: sizeBytes,
			SizeState: sizeState,
		})
	}

	return buildEvaluation(snapshot.EvaluatedAt, itemsByCategory)
}

func appendBuildCacheCandidate(items map[Category][]Item, caches []*types.BuildCache) {
	var size int64
	count := 0
	sizeState := SizeKnown
	for _, cache := range caches {
		if cache == nil || cache.InUse {
			continue
		}
		count++
		if cache.Size < 0 {
			sizeState = SizeUnknown
			continue
		}
		size += cache.Size
	}
	if count == 0 {
		return
	}
	items[CategoryBuildCache] = []Item{{
		Key:       itemKey(CategoryBuildCache, BuildCacheAggregateID),
		Category:  CategoryBuildCache,
		ID:        BuildCacheAggregateID,
		Name:      "未使用构建缓存",
		Detail:    buildCacheDetail(count),
		SizeBytes: size,
		SizeState: sizeState,
	}}
}

func buildEvaluation(evaluatedAt time.Time, itemsByCategory map[Category][]Item) Evaluation {
	evaluation := Evaluation{EvaluatedAt: evaluatedAt, Categories: make([]CategoryEvaluation, 0, len(categoryDefinitions))}
	for _, definition := range categoryDefinitions {
		items := itemsByCategory[definition.key]
		if items == nil {
			items = []Item{}
		}
		sort.Slice(items, func(i, j int) bool {
			if items[i].Name == items[j].Name {
				return items[i].ID < items[j].ID
			}
			return items[i].Name < items[j].Name
		})
		category := CategoryEvaluation{
			Key:             definition.key,
			Label:           definition.label,
			Risk:            definition.risk,
			DefaultSelected: definition.defaultSelected,
			Count:           len(items),
			Items:           items,
		}
		for _, item := range items {
			evaluation.Summary.CandidateCount++
			switch item.SizeState {
			case SizeKnown:
				category.KnownReclaimableBytes += item.SizeBytes
				evaluation.Summary.KnownReclaimableBytes += item.SizeBytes
			case SizeUnknown:
				category.UnknownSizeCount++
				evaluation.Summary.UnknownSizeCount++
			}
		}
		evaluation.Categories = append(evaluation.Categories, category)
	}
	return evaluation
}

func isStoppedContainerState(state string) bool {
	switch strings.ToLower(strings.TrimSpace(state)) {
	case "created", "exited", "dead":
		return true
	default:
		return false
	}
}

func isProtectedContainer(snapshot Snapshot, container types.Container) bool {
	if _, ok := snapshot.ProtectedContainerIDs[strings.TrimSpace(container.ID)]; ok {
		return true
	}
	for _, name := range container.Names {
		if _, ok := snapshot.ProtectedContainerNames[strings.TrimPrefix(strings.TrimSpace(name), "/")]; ok {
			return true
		}
	}
	return false
}

func isDanglingImage(image types.ImageSummary) bool {
	if len(image.RepoTags) == 0 {
		return true
	}
	for _, tag := range image.RepoTags {
		if tag != "<none>:<none>" && strings.TrimSpace(tag) != "" {
			return false
		}
	}
	return true
}

func isUnusedNetwork(network types.NetworkResource) bool {
	name := strings.ToLower(strings.TrimSpace(network.Name))
	if name == "bridge" || name == "host" || name == "none" || network.Ingress {
		return false
	}
	return strings.TrimSpace(network.ID) != "" && len(network.Containers) == 0
}

func firstContainerName(container types.Container) string {
	for _, name := range container.Names {
		if cleaned := strings.TrimPrefix(strings.TrimSpace(name), "/"); cleaned != "" {
			return cleaned
		}
	}
	return shortID(container.ID)
}

func firstImageName(image types.ImageSummary) string {
	for _, tag := range image.RepoTags {
		if tag != "<none>:<none>" && strings.TrimSpace(tag) != "" {
			return tag
		}
	}
	return shortID(image.ID)
}

func itemKey(category Category, id string) string {
	return string(category) + ":" + id
}

func shortID(id string) string {
	id = strings.TrimPrefix(strings.TrimSpace(id), "sha256:")
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

func maxInt64(value, minimum int64) int64 {
	if value < minimum {
		return minimum
	}
	return value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return "-"
}

func buildCacheDetail(count int) string {
	return fmt.Sprintf("%d 项", count)
}
