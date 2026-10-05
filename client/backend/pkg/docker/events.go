// backend/pkg/docker/events.go
package docker // 必须作为第一行

import (
	"context"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/client"
	"log"
)

// 使用正确的类型 events.Message
var BroadcastFunc func(events.Message)

func SetBroadcastHandler(fn func(events.Message)) {
	BroadcastFunc = fn
}

func WatchDockerEvents(cli *client.Client) {
	ctx := context.Background()
	eventsChan, errChan := cli.Events(ctx, types.EventsOptions{}) // 现在可以使用 types.EventsOptions

	for {
		select {
		case event := <-eventsChan:
			if BroadcastFunc != nil {
				BroadcastFunc(event)
			}
		case err := <-errChan:
			log.Printf("事件监听错误: %v", err)
			return
		}
	}
}
