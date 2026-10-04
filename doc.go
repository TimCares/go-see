// Package see records typed, semantic events instead of free form log lines.
//
// An event is the stable, machine readable tier of observability: something that
// happened in the application that is relevant enough to be queried, alerted on, or
// reported against in a durable way. Every event carries a dot separated [ID] (the
// vocabulary) plus a typed payload (its associated data), so that illegal events,
// e.g. a "caller transferred" without a target, are unrepresentable at construction.
//
// An event is not a log record. It is an occurrence that is projected onto sinks:
// today a log record and a set of metrics, tomorrow possibly a span. Each projection
// is a method on the event, so an event stays the single place that knows everything
// about one kind of occurrence:
//
//	var ArtifactUploadID = see.NewID("storage.artifact.upload")
//
//	type ArtifactUpload struct {
//		URI   string                   `json:"uri"`
//		Error *see.Error[UploadReason] `json:"error"` // No error means success.
//	}
//
//	func (ArtifactUpload) ID() see.ID { return ArtifactUploadID }
//
//	see.Emit(ctx, ArtifactUpload{URI: uri, Error: see.Fail(err, UploadNotStarted)})
//
// The event's id becomes the log message and the `event.id` field, its payload
// becomes the `event.data` group. Optional behaviour is opted into by implementing
// [Leveler] for severity, [Measurer] for metrics and [FriendlyMessenger] for a human
// readable log message.
//
// # Stability
//
// Event ids are a stable contract used during analytics, e.g. filtering or alerting.
// Changing or removing an existing id breaks dashboards and historical queries.
// Adding new events, and adding optional fields to existing events, is safe. Never
// repurpose an id or change a field's meaning under the same name.
//
// # Reserved ids
//
// The `see` root is owned by this package and used for meta observability, i.e.
// events about the recording of events:
//
//	see.emit.error     An event without a valid id was emitted.
//	see.measure.error  An event's Measure panicked, its metrics are lost.
//	see.message.error  An event's Message panicked, it is recorded under its id.
package see
