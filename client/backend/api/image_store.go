package api

import (
	"context"
	"fmt"
	"strings"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/errdefs"
)

type imageTagRemovalStore interface {
	ImageTag(context.Context, string, string) error
	ImageRemove(context.Context, string, types.ImageRemoveOptions) ([]types.ImageDeleteResponseItem, error)
}

func retainImageReferenceWithStore(ctx context.Context, store imageTagRemovalStore, source string, target string) error {
	if store == nil {
		return fmt.Errorf("Docker 镜像存储不可用")
	}
	source = strings.TrimSpace(source)
	target = strings.TrimSpace(target)
	if source == "" || target == "" {
		return fmt.Errorf("镜像引用不能为空")
	}
	return store.ImageTag(ctx, source, target)
}

func releaseImageReferenceWithStore(ctx context.Context, store imageTagRemovalStore, reference string) error {
	if store == nil {
		return fmt.Errorf("Docker 镜像存储不可用")
	}
	reference = strings.TrimSpace(reference)
	if reference == "" {
		return nil
	}
	// Force only removes this temporary tag. Docker preserves an image that is
	// still referenced by another tag or a container.
	if _, err := store.ImageRemove(ctx, reference, types.ImageRemoveOptions{Force: true, PruneChildren: false}); err != nil {
		if errdefs.IsNotFound(err) {
			return nil
		}
		return err
	}
	return nil
}
