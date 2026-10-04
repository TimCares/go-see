package see

import (
	"context"

	"go.uber.org/zap/zapcore"
)

// Event is anything that can be recorded via [Emit].
//
// The event's exported fields are its payload, encoded through their `json` tags.
// Events on a hot path can implement [zapcore.ObjectMarshaler] to encode their
// payload without reflection.
//
// Example event:
//
//	type CallerTransferred struct {
//		TransferTo string `json:"transfer_to"`
//	}
//
//	func (CallerTransferred) ID() see.ID { return CallerTransferredID }
type Event interface {
	ID() ID
}

// Leveler is implemented by events that are not an unremarkable [LevelInfo].
//
// Severity is a property of the occurrence, not of the call site: the same event
// emitted from two places must not end up at two different levels, or alerting on
// it silently splits.
type Leveler interface {
	Level() Level
}

// Measurer is implemented by events that also project onto metrics.
//
// Strictly for metrics of the form "how often did this happen". Anything without a
// one to one correspondence to an occurrence, a latency histogram, a queue depth
// gauge, ..., has no business here and should be written to its instrument directly.
//
// Measure runs after the log record is written, as the record is the primary signal.
// A panic, e.g. from a broken label set, is recovered and reported as
// "see.measure.error", so it degrades the metric rather than the event.
type Measurer interface {
	Measure(ctx context.Context)
}

// FriendlyMessenger is implemented by events that have a human readable, plain language string.
//
// Message fills the MessageKey field of the [zapcore.Logger].
// Events that are not a [FriendlyMessenger], or return an empty message, have [ID] as
// their log message.
//
// Message is only called if the event's level is enabled. A panic is recovered and
// reported as "see.message.error", the event is then recorded with [ID] as its message.
type FriendlyMessenger interface {
	Message() string
}

// Level is how bad an occurrence is.
//
// Deliberately narrower than zap's levels: recording an event must never panic or
// exit the process, so there is no equivalent of DPanic, Panic or Fatal.
type Level int8

// The values match zapcore's, so the zero Level is info.
const (
	LevelDebug Level = iota - 1
	LevelInfo
	LevelWarn
	LevelError
)

func (l Level) zapLevel() zapcore.Level {
	// Clamped, as an out of range conversion like Level(5) would otherwise be fatal.
	return zapcore.Level(min(max(l, LevelDebug), LevelError))
}

func levelOf(event Event) zapcore.Level {
	if leveler, ok := event.(Leveler); ok {
		return leveler.Level().zapLevel()
	}
	return zapcore.InfoLevel
}
