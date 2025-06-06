package mqueue

import (
	"app/base/utils"
	"time"

	"github.com/bytedance/sonic"
	"github.com/lestrrat-go/backoff/v2"
	"golang.org/x/net/context"
)

var BatchSize = utils.PodConfig.GetInt("msg_batch_size", 4000)

var policy = backoff.Exponential(
	backoff.WithMinInterval(time.Second),
	backoff.WithMaxRetries(5),
)

type EventHandler func(message PlatformEvent) error

type MessageData interface {
	WriteEvents(ctx context.Context, w Writer) error
}

// Performs parsing of kafka message, and then dispatches this message into provided functions
func MakeMessageHandler(eventHandler EventHandler) MessageHandler {
	return func(m KafkaMessage) error {
		var event PlatformEvent
		err := sonic.Unmarshal(m.Value, &event)
		// Not a fatal error, invalid data format, log and skip
		if err != nil {
			utils.LogError("err", err.Error(), "Could not deserialize platform event")
			return nil
		}
		return eventHandler(event)
	}
}

func SendMessages(ctx context.Context, w Writer, data MessageData) error {
	return data.WriteEvents(ctx, w)
}
