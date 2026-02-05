package connector

import (
	"context"

	"maunium.net/go/mautrix/event"
)

type statusEventsKey struct{}
type queueAcceptedStatusKey struct{}

func withStatusEvents(ctx context.Context, events []*event.Event) context.Context {
	if len(events) == 0 {
		return ctx
	}
	return context.WithValue(ctx, statusEventsKey{}, events)
}

func statusEventsFromContext(ctx context.Context) []*event.Event {
	if ctx == nil {
		return nil
	}
	if raw := ctx.Value(statusEventsKey{}); raw != nil {
		if events, ok := raw.([]*event.Event); ok {
			return events
		}
	}
	return nil
}

func withQueueAcceptedStatus(ctx context.Context) context.Context {
	return context.WithValue(ctx, queueAcceptedStatusKey{}, true)
}

func queueAcceptedStatusFromContext(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	raw := ctx.Value(queueAcceptedStatusKey{})
	accepted, ok := raw.(bool)
	return ok && accepted
}
