package see

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	testUploadID    = NewID("test.emit.upload")
	testLevelID     = NewID("test.emit.level")
	testMarshaledID = NewID("test.emit.marshaled")
	testFriendlyID  = NewID("test.emit.friendly")
)

type testUploadReason string

const testUploadNotStarted testUploadReason = "not_started"

type testUpload struct {
	URI   string                   `json:"uri"`
	Error *Error[testUploadReason] `json:"error"`

	measure func()
}

func (testUpload) ID() ID { return testUploadID }

func (e testUpload) Level() Level {
	if e.Error != nil {
		return LevelWarn
	}
	return LevelInfo
}

func (e testUpload) Measure(context.Context) {
	if e.measure != nil {
		e.measure()
	}
}

type testLevel struct {
	level Level
}

func (testLevel) ID() ID { return testLevelID }

func (e testLevel) Level() Level { return e.level }

type testMarshaled struct {
	Name string
}

func (testMarshaled) ID() ID { return testMarshaledID }

func (e testMarshaled) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	enc.AddString("marshaled_name", e.Name)
	return nil
}

type testFriendly struct {
	URI string `json:"uri"`

	message func() string
}

func (testFriendly) ID() ID { return testFriendlyID }

func (e testFriendly) Message() string { return e.message() }

type testNoID struct{}

func (testNoID) ID() ID { return ID{} }

// newTestEmitter returns an emitter writing json records, one per line, to the returned buffer.
func newTestEmitter(t *testing.T) (*Emitter, *bytes.Buffer) {
	t.Helper()
	return newTestEmitterAt(t, zapcore.DebugLevel)
}

// newTestEmitterAt is [newTestEmitter], with every record below level discarded.
func newTestEmitterAt(t *testing.T, level zapcore.Level) (*Emitter, *bytes.Buffer) {
	t.Helper()

	buf := &bytes.Buffer{}
	encoder := zapcore.NewJSONEncoder(zapcore.EncoderConfig{
		MessageKey:   "msg",
		LevelKey:     "level",
		CallerKey:    "caller",
		EncodeLevel:  zapcore.LowercaseLevelEncoder,
		EncodeCaller: zapcore.ShortCallerEncoder,
	})
	logger := zap.New(zapcore.NewCore(encoder, zapcore.AddSync(buf), level), zap.AddCaller())

	emitter, err := New(Config{Logger: logger})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return emitter, buf
}

func decodeRecords(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()

	var records []map[string]any
	for line := range strings.Lines(buf.String()) {
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("decode %q: %v", line, err)
		}
		records = append(records, record)
	}
	return records
}

func decodeRecord(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()

	records := decodeRecords(t, buf)
	if len(records) != 1 {
		t.Fatalf("got %d records, want 1: %v", len(records), records)
	}
	return records[0]
}

// path walks nested json objects, e.g. path(record, "event", "data", "uri").
func path(t *testing.T, record map[string]any, keys ...string) any {
	t.Helper()

	var value any = record
	for _, key := range keys {
		object, ok := value.(map[string]any)
		if !ok {
			t.Fatalf("%v is not an object at %q in %v", value, key, record)
		}
		value = object[key]
	}
	return value
}

func TestEmitRecordsTheEventAsOneRecord(t *testing.T) {
	emitter, buf := newTestEmitter(t)

	ctx := With(context.Background(), zap.String("session_id", "abc"))
	emitter.Emit(ctx, testUpload{URI: "s3://bucket/recording"})

	record := decodeRecord(t, buf)
	want := map[string]any{
		"msg":        "test.emit.upload",
		"level":      "info",
		"session_id": "abc",
		"event": map[string]any{
			"id":   "test.emit.upload",
			"data": map[string]any{"uri": "s3://bucket/recording", "error": nil},
		},
	}
	for key, value := range want {
		if got, _ := json.Marshal(record[key]); string(got) != mustJSON(t, value) {
			t.Errorf("%s = %s, want %s", key, got, mustJSON(t, value))
		}
	}
}

func TestEmitReportsTheCallSiteAsCaller(t *testing.T) {
	emitter, buf := newTestEmitter(t)

	emitter.Emit(context.Background(), testUpload{})
	SetDefault(emitter)
	t.Cleanup(func() { SetDefault(nil) })
	Emit(context.Background(), testUpload{})

	for _, record := range decodeRecords(t, buf) {
		if caller, _ := record["caller"].(string); !strings.Contains(caller, "emit_test.go") {
			t.Errorf("caller = %q, want the call site in emit_test.go", caller)
		}
	}
}

func TestEmitRecordsTheError(t *testing.T) {
	emitter, buf := newTestEmitter(t)

	emitter.Emit(context.Background(), testUpload{
		URI:   "s3://bucket/recording",
		Error: Fail(errors.New("connection refused"), testUploadNotStarted),
	})

	record := decodeRecord(t, buf)
	if got := record["level"]; got != "warn" {
		t.Errorf("level = %v, want warn", got)
	}
	if got := path(t, record, "event", "data", "error", "raw_error"); got != "connection refused" {
		t.Errorf("raw_error = %v, want connection refused", got)
	}
	if got := path(t, record, "event", "data", "error", "stable_error_reason"); got != "not_started" {
		t.Errorf("stable_error_reason = %v, want not_started", got)
	}
}

func TestEmitClampsTheLevel(t *testing.T) {
	cases := map[Level]string{
		LevelDebug: "debug",
		LevelInfo:  "info",
		LevelWarn:  "warn",
		LevelError: "error",
		Level(5):   "error", // Fatal in zap, which would exit the test binary.
		Level(-3):  "debug",
	}
	for level, want := range cases {
		emitter, buf := newTestEmitter(t)
		emitter.Emit(context.Background(), testLevel{level: level})

		if got := decodeRecord(t, buf)["level"]; got != want {
			t.Errorf("Level(%d) recorded as %v, want %s", level, got, want)
		}
	}
}

func TestEmitUsesTheObjectMarshaler(t *testing.T) {
	emitter, buf := newTestEmitter(t)

	emitter.Emit(context.Background(), testMarshaled{Name: "fast"})

	if got := path(t, decodeRecord(t, buf), "event", "data", "marshaled_name"); got != "fast" {
		t.Errorf("marshaled_name = %v, want fast", got)
	}
}

func TestEmitUsesTheFriendlyMessage(t *testing.T) {
	emitter, buf := newTestEmitter(t)

	emitter.Emit(context.Background(), testFriendly{
		URI:     "s3://bucket/recording",
		message: func() string { return "uploaded s3://bucket/recording" },
	})

	record := decodeRecord(t, buf)
	if got := record["msg"]; got != "uploaded s3://bucket/recording" {
		t.Errorf("msg = %v, want uploaded s3://bucket/recording", got)
	}
	if got := path(t, record, "event", "id"); got != "test.emit.friendly" {
		t.Errorf("event.id = %v, want test.emit.friendly", got)
	}
	if got := path(t, record, "event", "data"); mustJSON(t, got) != `{"uri":"s3://bucket/recording"}` {
		t.Errorf("event.data = %s, want only the payload", mustJSON(t, got))
	}
}

func TestEmitFallsBackToTheIDForAnEmptyMessage(t *testing.T) {
	emitter, buf := newTestEmitter(t)

	emitter.Emit(context.Background(), testFriendly{message: func() string { return "" }})

	if got := decodeRecord(t, buf)["msg"]; got != "test.emit.friendly" {
		t.Errorf("msg = %v, want test.emit.friendly", got)
	}
}

func TestEmitSkipsTheMessageForADisabledLevel(t *testing.T) {
	emitter, buf := newTestEmitterAt(t, zapcore.WarnLevel)

	called := false
	emitter.Emit(context.Background(), testFriendly{message: func() string {
		called = true
		return "expensive"
	}})

	if called {
		t.Error("Message called for a disabled level")
	}
	if records := decodeRecords(t, buf); len(records) != 0 {
		t.Errorf("got %d records, want none: %v", len(records), records)
	}
}

func TestEmitReportsAPanickingMessage(t *testing.T) {
	emitter, buf := newTestEmitter(t)

	emitter.Emit(context.Background(), testFriendly{
		URI:     "s3://bucket/recording",
		message: func() string { panic("nil pointer in message") },
	})

	records := decodeRecords(t, buf)
	if len(records) != 2 {
		t.Fatalf("got %d records, want the event and its message failure: %v", len(records), records)
	}

	event := records[0]
	if got := event["msg"]; got != "test.emit.friendly" {
		t.Errorf("event msg = %v, want test.emit.friendly", got)
	}
	if got := path(t, event, "event", "data", "uri"); got != "s3://bucket/recording" {
		t.Errorf("uri = %v, want s3://bucket/recording", got)
	}

	failure := records[1]
	if got := failure["msg"]; got != "see.message.error" {
		t.Errorf("msg = %v, want see.message.error", got)
	}
	if got := failure["level"]; got != "warn" {
		t.Errorf("level = %v, want warn", got)
	}
	if caller, _ := failure["caller"].(string); !strings.Contains(caller, "emit_test.go") {
		t.Errorf("caller = %q, want the call site in emit_test.go", caller)
	}
	if got := path(t, failure, "event", "data", "event_id"); got != "test.emit.friendly" {
		t.Errorf("event_id = %v, want test.emit.friendly", got)
	}
	if got := path(t, failure, "event", "data", "error", "stable_error_reason"); got != "panic" {
		t.Errorf("stable_error_reason = %v, want panic", got)
	}
	if got, _ := path(t, failure, "event", "data", "error", "raw_error").(string); !strings.Contains(got, "nil pointer in message") {
		t.Errorf("raw_error = %q, want the panic value", got)
	}
}

func TestEmitMeasuresOnce(t *testing.T) {
	emitter, _ := newTestEmitter(t)

	calls := 0
	emitter.Emit(context.Background(), testUpload{measure: func() { calls++ }})

	if calls != 1 {
		t.Errorf("Measure called %d times, want 1", calls)
	}
}

func TestEmitReportsAPanickingMeasure(t *testing.T) {
	emitter, buf := newTestEmitter(t)

	emitter.Emit(context.Background(), testUpload{measure: func() { panic("inconsistent label cardinality") }})

	records := decodeRecords(t, buf)
	if len(records) != 2 {
		t.Fatalf("got %d records, want the event and its measure failure: %v", len(records), records)
	}

	failure := records[1]
	if got := failure["msg"]; got != "see.measure.error" {
		t.Errorf("msg = %v, want see.measure.error", got)
	}
	if got := failure["level"]; got != "warn" {
		t.Errorf("level = %v, want warn", got)
	}
	if got := path(t, failure, "event", "data", "event_id"); got != "test.emit.upload" {
		t.Errorf("event_id = %v, want test.emit.upload", got)
	}
	if got := path(t, failure, "event", "data", "error", "stable_error_reason"); got != "panic" {
		t.Errorf("stable_error_reason = %v, want panic", got)
	}
	if got, _ := path(t, failure, "event", "data", "error", "raw_error").(string); !strings.Contains(got, "inconsistent label cardinality") {
		t.Errorf("raw_error = %q, want the panic value", got)
	}
}

func TestEmitReportsAMissingID(t *testing.T) {
	cases := map[string]Event{
		"see.testNoID": testNoID{},
		"<nil>":        nil,
	}
	for eventType, event := range cases {
		emitter, buf := newTestEmitter(t)
		emitter.Emit(context.Background(), event)

		record := decodeRecord(t, buf)
		if got := record["msg"]; got != "see.emit.error" {
			t.Errorf("msg = %v, want see.emit.error", got)
		}
		if got := record["level"]; got != "error" {
			t.Errorf("level = %v, want error", got)
		}
		if got := path(t, record, "event", "data", "event_type"); got != eventType {
			t.Errorf("event_type = %v, want %s", got, eventType)
		}
	}
}

func TestWithDoesNotLeakBetweenSiblings(t *testing.T) {
	parent := With(context.Background(), zap.String("a", "1"))
	left := With(parent, zap.String("b", "2"))
	right := With(parent, zap.String("c", "3"))

	keys := func(ctx context.Context) (keys []string) {
		for _, field := range contextFields(ctx) {
			keys = append(keys, field.Key)
		}
		return keys
	}
	if got := strings.Join(keys(left), ","); got != "a,b" {
		t.Errorf("left = %s, want a,b", got)
	}
	if got := strings.Join(keys(right), ","); got != "a,c" {
		t.Errorf("right = %s, want a,c", got)
	}
}

func TestNewRequiresALogger(t *testing.T) {
	if _, err := New(Config{}); err == nil {
		t.Error("New(Config{}) succeeded, want an error")
	}
}

func TestDefaultDiscardsUntilSet(t *testing.T) {
	if Default() != nop {
		t.Fatal("Default() is not the discarding emitter")
	}
	Emit(context.Background(), testUpload{})
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()

	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("encode %v: %v", value, err)
	}
	return string(encoded)
}

func TestDefaultSetsPlainZapLogger(t *testing.T) {
	emitter, buf := newTestEmitter(t)
	SetDefault(emitter)
	t.Cleanup(func() { SetDefault(nil) })

	ctx := With(context.Background(), zap.String("session_id", "abc"))
	L(ctx).Info("hello")

	record := decodeRecord(t, buf)
	if got := record["msg"]; got != "hello" {
		t.Errorf("msg = %v, want hello", got)
	}
	if got := record["session_id"]; got != "abc" {
		t.Errorf("session_id = %v, want abc", got)
	}
	if got, ok := record["event"]; ok {
		t.Errorf("event = %v, want no event group on a plain log", got)
	}
}

func TestPlainZapLoggerCorrectCallerLocation(t *testing.T) {
	emitter, buf := newTestEmitter(t)
	SetDefault(emitter)
	t.Cleanup(func() { SetDefault(nil) })

	_, _, line, _ := runtime.Caller(0)
	L(context.Background()).Info("hello")

	want := fmt.Sprintf("emit_test.go:%d", line+1)
	if caller, _ := decodeRecord(t, buf)["caller"].(string); !strings.HasSuffix(caller, want) {
		t.Errorf("caller = %q, want the call site %s", caller, want)
	}
}

func TestSetDefaultNilRestoresThePlainZapLogger(t *testing.T) {
	emitter, buf := newTestEmitter(t)
	SetDefault(emitter)
	SetDefault(nil)

	if Default() != nop {
		t.Fatal("Default() is not the discarding emitter")
	}
	L(context.Background()).Info("dropped")
	if records := decodeRecords(t, buf); len(records) != 0 {
		t.Errorf("got %d records, want none: %v", len(records), records)
	}
}
