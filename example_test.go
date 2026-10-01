package see_test

import (
	"context"
	"errors"
	"expvar"
	"fmt"
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/TimCares/go-see"
)

var ArtifactUploadID = see.NewID("storage.artifact.upload")

// Stand ins for labelled Prometheus or OpenTelemetry counters.
var (
	artifactUploadsTotal      = expvar.NewMap("artifact_uploads_total")
	artifactUploadErrorsTotal = expvar.NewMap("artifact_upload_errors_total")
)

// ArtifactType is the artifact a call produces.
type ArtifactType string

const (
	Recording           ArtifactType = "recording"
	ConversationHistory ArtifactType = "conversation_history"
)

// ArtifactUploadErrorReason is every reason an upload can fail with.
type ArtifactUploadErrorReason string

const (
	ArtifactUploadTimeout    ArtifactUploadErrorReason = "timeout"
	ArtifactUploadNotStarted ArtifactUploadErrorReason = "not_started" // The upload failed to start or was never initiated.
)

// ArtifactUpload records that one artifact was persisted to object storage, or failed to be.
//
// An error is identified by Error being set, URI still carries the location the
// artifact was meant to occupy, so a missing artifact can be traced back.
type ArtifactUpload struct {
	ArtifactType ArtifactType                          `json:"artifact_type"`
	URI          string                                `json:"uri"`
	Error        *see.Error[ArtifactUploadErrorReason] `json:"error"` // No error means success.
}

func (ArtifactUpload) ID() see.ID { return ArtifactUploadID }

// Level reports a failed upload as a warning: the call itself still succeeded.
func (e ArtifactUpload) Level() see.Level {
	if e.Error != nil {
		return see.LevelWarn
	}
	return see.LevelInfo
}

// Measure counts the attempt, and separately counts it if it failed.
func (e ArtifactUpload) Measure(context.Context) {
	artifactUploadsTotal.Add(string(e.ArtifactType), 1)
	if e.Error != nil {
		artifactUploadErrorsTotal.Add(string(e.ArtifactType)+","+string(e.Error.Reason), 1)
	}
}

func Example() {
	encoder := zapcore.NewJSONEncoder(zapcore.EncoderConfig{
		MessageKey:  "msg",
		LevelKey:    "level",
		EncodeLevel: zapcore.LowercaseLevelEncoder,
	})
	logger := zap.New(zapcore.NewCore(encoder, zapcore.AddSync(os.Stdout), zapcore.InfoLevel))

	emitter, err := see.New(see.Config{Logger: logger})
	if err != nil {
		panic(err)
	}
	see.SetDefault(emitter)
	defer see.SetDefault(nil)

	ctx := see.With(context.Background(), zap.String("session_id", "abc"))

	see.Emit(ctx, ArtifactUpload{
		ArtifactType: Recording,
		URI:          "s3://calls/abc/recording.ogg",
		Error:        nil,
	})

	see.Emit(ctx, ArtifactUpload{
		ArtifactType: ConversationHistory,
		URI:          "s3://calls/abc/history.json",
		Error:        see.Fail(errors.New("context deadline exceeded"), ArtifactUploadTimeout),
	})

	fmt.Println(artifactUploadsTotal)
	fmt.Println(artifactUploadErrorsTotal)

	see.L(ctx).Info("hello")

	// Output:
	// {"level":"info","msg":"storage.artifact.upload","session_id":"abc","event":{"id":"storage.artifact.upload","data":{"artifact_type":"recording","uri":"s3://calls/abc/recording.ogg","error":null}}}
	// {"level":"warn","msg":"storage.artifact.upload","session_id":"abc","event":{"id":"storage.artifact.upload","data":{"artifact_type":"conversation_history","uri":"s3://calls/abc/history.json","error":{"raw_error":"context deadline exceeded","stable_error_reason":"timeout"}}}}
	// {"conversation_history": 1, "recording": 1}
	// {"conversation_history,timeout": 1}
	// {"level":"info","msg":"hello","session_id":"abc"}
}
