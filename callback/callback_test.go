package callback

import (
	"errors"
	"testing"

	"github.com/cloudwego/eino/schema"
)

func TestCollectStreamPropagatesError(t *testing.T) {
	reader, writer := schema.Pipe[*schema.Message](2)
	defer reader.Close()
	failure := errors.New("upstream disconnected")
	writer.Send(schema.AssistantMessage("partial", nil), nil)
	writer.Send(nil, failure)
	writer.Close()
	done := false
	content, err := CollectStream(reader, &StreamCallback{OnDone: func(string) { done = true }})
	if content != "partial" || !errors.Is(err, failure) || done {
		t.Fatalf("content=%q err=%v done=%v", content, err, done)
	}
}
