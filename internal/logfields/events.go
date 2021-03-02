package logfields

import "go.uber.org/zap"

func EventProvider(val string) zap.Field {
	return zap.String("event_provider", val)
}

func Event(val string) zap.Field {
	return zap.String("event", val)
}
