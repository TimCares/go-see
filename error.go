package see

import (
	"encoding/json"

	"go.uber.org/zap/zapcore"
)

// Error is a reusable payload field for events whose operation can fail.
//
// Err is a catch all for any error information. Useful to not swallow important
// information that cannot, and should not, be represented by a stable and concise
// vocabulary. It is recorded as `raw_error`.
//
// Reason is a stable vocabulary to classify errors, which is excellent for queries
// and filtering. It is recorded as `stable_error_reason`. Parameterise it with one
// string type per domain, holding every reason the operation can fail with:
//
//	type UploadReason string
//
//	const (
//		UploadTimeout    UploadReason = "timeout"
//		UploadNotStarted UploadReason = "not_started"
//	)
type Error[R ~string] struct {
	Err    error
	Reason R // Better to query and filter on than a raw error string.
}

// Fail builds an [Error] with the reason type inferred from reason.
func Fail[R ~string](err error, reason R) *Error[R] {
	return &Error[R]{Err: err, Reason: reason}
}

// MarshalJSON encodes the error for reflection based payloads.
func (e Error[R]) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		RawError          string `json:"raw_error"`
		StableErrorReason R      `json:"stable_error_reason"`
	}{
		RawError:          e.raw(),
		StableErrorReason: e.Reason,
	})
}

// MarshalLogObject encodes the error for events that implement [zapcore.ObjectMarshaler].
func (e Error[R]) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	enc.AddString("raw_error", e.raw())
	enc.AddString("stable_error_reason", string(e.Reason))
	return nil
}

func (e Error[R]) raw() string {
	if e.Err == nil {
		return ""
	}
	return e.Err.Error()
}
