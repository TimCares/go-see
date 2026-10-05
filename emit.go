package see

import (
	"context"
	"errors"
	"fmt"
	"runtime/debug"
	"slices"
	"sync/atomic"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Config is everything an [Emitter] needs to record events.
type Config struct {
	// Logger receives one record per event. Required.
	//
	// Its encoder, sinks and level are left untouched, so the records look like any
	// other record of the application. Enable zap.AddCaller on it to have the call
	// site of Emit, not this package, reported as the caller.
	Logger *zap.Logger
}

// Emitter fans an event out to every sink it projects onto.
type Emitter struct {
	logger *zap.Logger
	// direct is the configured logger without the caller skip of logger, so [L]
	// reports its own call site.
	direct *zap.Logger
}

// New builds an [Emitter] from cfg.
func New(cfg Config) (*Emitter, error) {
	if cfg.Logger == nil {
		return nil, errors.New("see: Config.Logger is required")
	}

	// Frame 0 is record, frame 1 is emit, frame 2 is Emit, frame 3 is the caller.
	return &Emitter{logger: cfg.Logger.WithOptions(zap.AddCallerSkip(3)), direct: cfg.Logger}, nil
}

// Emit records event on every sink it projects onto.
func (e *Emitter) Emit(ctx context.Context, event Event) {
	e.emit(ctx, event)
}

var (
	nop            = &Emitter{logger: zap.NewNop(), direct: zap.NewNop()}
	defaultEmitter atomic.Pointer[Emitter]
)

// Default returns the [Emitter] used by [Emit].
//
// Until [SetDefault] is called, it discards every event, so libraries can emit
// before, or without, the application configuring anything.
func Default() *Emitter {
	if e := defaultEmitter.Load(); e != nil {
		return e
	}
	return nop
}

// SetDefault makes e the [Emitter] used by [Emit]. A nil e restores the discarding one.
func SetDefault(e *Emitter) {
	defaultEmitter.Store(e)
}

// Emit records event on every sink it projects onto, using the [Default] emitter.
//
// The event's [FriendlyMessenger.Message], or its id, becomes the log message, its id
// becomes the `event.id` field, its payload becomes the `event.data` group, and every
// field attached to ctx through [With] is added alongside.
func Emit(ctx context.Context, event Event) {
	Default().emit(ctx, event)
}

// L can be used to access the backing [zap] logger directly.
//
// Attaches the same context just like [Emit], but without a typed event, and reports
// the call site of the log call as the caller.
func L(ctx context.Context) *zap.Logger {
	return Default().direct.With(contextFields(ctx)...)
}

type fieldsKey struct{}

// With returns a copy of ctx that attaches fields to every event emitted with it.
//
// This is the place for ambient context, e.g. a session or request id, so that it
// does not have to be repeated in every event's payload.
func With(ctx context.Context, fields ...zap.Field) context.Context {
	parent := contextFields(ctx)
	return context.WithValue(ctx, fieldsKey{}, append(slices.Clip(parent), fields...))
}

func contextFields(ctx context.Context) []zap.Field {
	fields, _ := ctx.Value(fieldsKey{}).([]zap.Field)
	return fields
}

func (e *Emitter) emit(ctx context.Context, event Event) {
	if event == nil || event.ID() == (ID{}) {
		e.record(ctx, emitFailure{
			EventType: fmt.Sprintf("%T", event),
			Error:     Fail(errors.New("event has no id, ids are created with see.NewID"), emitErrorMissingID),
		})
		return
	}

	if failure := e.record(ctx, event); failure != nil {
		// Not routed through emit, a failure event never measures itself.
		e.record(ctx, failure)
	}

	if measurer, ok := event.(Measurer); ok {
		if failure := measure(ctx, event.ID(), measurer); failure != nil {
			// Not routed through emit, a failure event never measures itself.
			e.record(ctx, failure)
		}
	}
}

// logMessage gets the message put into the message field of the [zapcore.Logger].
//
// If the [Event] is a [FriendlyMessenger] we take the friendly message [FriendlyMessenger.Message],
// otherwise we use the [Event.ID].
//
// If [FriendlyMessenger.Message] returns an empty string or panics we fall back to [Event.ID],
// a panic is reported instead of propagated.
func logMessage(event Event) (message string, isFriendly bool, failure *messageFailure) {
	friendlyMessenger, ok := event.(FriendlyMessenger)
	if !ok {
		return event.ID().String(), false, nil
	}

	defer func() {
		if r := recover(); r != nil {
			message = event.ID().String()
			failure = &messageFailure{
				EventID:    event.ID(),
				Error:      Fail(fmt.Errorf("message panicked: %v", r), messageErrorPanic),
				StackTrace: string(debug.Stack()),
			}
		}
	}()

	if message = friendlyMessenger.Message(); message == "" {
		return event.ID().String(), false, nil // Fallback to event id if friendly message is empty.
	}
	return message, true, nil
}

// record projects the event onto the log sink, and returns the failure of a panicking Message.
func (e *Emitter) record(ctx context.Context, event Event) *messageFailure {
	level := levelOf(event)
	// Checked before building the message, so a disabled level never calls Message.
	if !e.logger.Core().Enabled(level) {
		return nil
	}

	message, isFriendly, failure := logMessage(event)
	if entry := e.logger.Check(level, message); entry != nil {
		fields := contextFields(ctx)
		eventObject := EventObject{
			event:      event,
			isFriendly: isFriendly,
		}
		entry.Write(append(slices.Clip(fields), zap.Object("event", eventObject))...)
	}
	return failure
}

// measure projects the event onto its metrics, and reports a panic instead of propagating it.
func measure(ctx context.Context, id ID, measurer Measurer) (failure *measureFailure) {
	defer func() {
		if r := recover(); r != nil {
			failure = &measureFailure{
				EventID:    id,
				Error:      Fail(fmt.Errorf("measure panicked: %v", r), measureErrorPanic),
				StackTrace: string(debug.Stack()),
			}
		}
	}()

	measurer.Measure(ctx)
	return nil
}

// EventObject groups an event into `{"id": ..., "data": ...}`.
type EventObject struct {
	event      Event
	isFriendly bool // msg is the event's own Message, not the id fallback.
}

// Event return the actual user-defined [Event] passed to [Emit].
func (o EventObject) Event() Event { return o.event }

// IsFriendly reports if the message of the [Event] landing in the log is friendly or not.
//
// An [Event] can report an "unfriendly" message even if it implements [FriendlyMessenger].
// This is either the result of [FriendlyMessenger.Message] panicing or returning an empty string.
//
// In both cases [Event.ID] is used as the fallback, which is machine readable, but not natural
// language, and the reason why this will return [false].
func (o EventObject) IsFriendly() bool { return o.isFriendly }

// MarshalLogObject writes the [EventObject.event] without the [EventObject.isFriendly] flag.
func (o EventObject) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	enc.AddString("id", o.event.ID().String())
	if marshaler, ok := o.event.(zapcore.ObjectMarshaler); ok {
		return enc.AddObject("data", marshaler)
	}
	return enc.AddReflected("data", o.event)
}

// Meta observability: events about the recording of events.

var (
	emitErrorID    = register("see.emit.error")
	measureErrorID = register("see.measure.error")
	messageErrorID = register("see.message.error")
)

type emitErrorReason string

const emitErrorMissingID emitErrorReason = "missing_id"

// emitFailure is recorded in place of an event that has no valid id.
type emitFailure struct {
	EventType string                  `json:"event_type"`
	Error     *Error[emitErrorReason] `json:"error"`
}

func (emitFailure) ID() ID { return emitErrorID }

// Level reports a missing id as an error: the occurrence itself is lost.
func (emitFailure) Level() Level { return LevelError }

type measureErrorReason string

// A metric is either counted or it is not, there is nothing finer to say than that it panicked.
const measureErrorPanic measureErrorReason = "panic"

// measureFailure is recorded when an event's Measure panicked.
type measureFailure struct {
	EventID    ID                         `json:"event_id"`
	Error      *Error[measureErrorReason] `json:"error"`
	StackTrace string                     `json:"stack_trace"`
}

func (measureFailure) ID() ID { return measureErrorID }

// Level reports a failed measure as a warning: not critical, but worth noting and fixing.
func (measureFailure) Level() Level { return LevelWarn }

type messageErrorReason string

const messageErrorPanic messageErrorReason = "panic"

// messageFailure is recorded when an event's Message panicked.
type messageFailure struct {
	EventID    ID                         `json:"event_id"`
	Error      *Error[messageErrorReason] `json:"error"`
	StackTrace string                     `json:"stack_trace"`
}

func (messageFailure) ID() ID { return messageErrorID }

// Level reports a failed message as a warning: the event is still recorded, under its id.
func (messageFailure) Level() Level { return LevelWarn }
