// Package callback provides CLI stream output.
package callback

import (
	"github.com/cloudwego/eino/schema"
	"io"
)

// StreamCallback 流式回调
type StreamCallback struct {
	OnChunk func(content string)
	OnDone  func(fullContent string)
}

// CollectStream 收集流式响应
func CollectStream(stream *schema.StreamReader[*schema.Message], cb *StreamCallback) (string, error) {
	var fullContent string

	for {
		chunk, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fullContent, err
		}
		if chunk != nil && chunk.Content != "" {
			fullContent += chunk.Content
			if cb != nil && cb.OnChunk != nil {
				cb.OnChunk(chunk.Content)
			}
		}
	}

	if cb != nil && cb.OnDone != nil {
		cb.OnDone(fullContent)
	}

	return fullContent, nil
}
