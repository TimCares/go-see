# Motivation

## Logs describe, events declare

Most logging looks like this:

```go
log.Info("uploaded artifact", zap.String("uri", uri))
log.Error("failed to upload artifact: " + err.Error())
log.Warn("artifact upload did not start", zap.String("type", kind))
```

It reads fine, and it is almost useless once you need to query it.

**It is not consistent.** Three messages describe the same occurrence, each phrased by
whoever wrote that call site. There is no single thing to search for.

**It is not explicit.** Nothing says which fields an upload carries, or how bad a failed
one is. The level is picked at the call site, so the same failure is an error in one
place and a warning in another, and an alert on it silently misses half the cases.

**It is not machine readable.** Finding every failed upload means grepping prose.
Rewording a message silently breaks every dashboard built on it, and interpolating
`err.Error()` makes each message unique, so nothing can be grouped at all.

## An event is a contract

go-see replaces the message with an **id** and the loose fields with a **typed payload**:

```go
var ArtifactUploadID = see.NewID("storage.artifact.upload")

type ArtifactUpload struct {
	ArtifactType ArtifactType             `json:"artifact_type"`
	URI          string                   `json:"uri"`
	Error        *see.Error[UploadReason] `json:"error"` // No error means success.
}
```

The id is the vocabulary. It is validated at startup, never changes, and is exactly
what you query and alert on. The payload is checked by the compiler, so an upload
without a URI, or a failure without a reason, cannot be emitted in the first place.

A query then stops being a guess:

```
event.id: "storage.artifact.upload" AND event.data.error.stable_error_reason: "timeout"
```

## One occurrence, one place

An occurrence is rarely only a log line. A failed upload is also a counter, and its
severity is a decision that should be made once. Without events, all of that lands in
the business logic:

```go
if err != nil {
	log.Warn("artifact upload failed", zap.String("uri", uri), zap.Error(err))
	uploadsTotal.WithLabelValues(kind).Inc()
	uploadErrorsTotal.WithLabelValues(kind, "timeout").Inc()
	return err
}
log.Info("artifact uploaded", zap.String("uri", uri))
uploadsTotal.WithLabelValues(kind).Inc()
```

With go-see, the event type owns its log record, its level and its metrics. The
business logic states what happened, and nothing else:

```go
upload := ArtifactUpload{ArtifactType: kind, URI: uri}
if err != nil {
	upload.Error = see.Fail(err, UploadTimeout)
}
see.Emit(ctx, upload)
```

Because every projection is derived from the same value, the log and the metrics can
never tell different stories.

## Stable reasons, raw errors

An error message is unbounded: it contains paths, ids, and whatever the library below
decided to say today. That makes it useful for a human and useless for a filter.

`see.Error` therefore records both. `raw_error` keeps every detail, so nothing is
swallowed. `stable_error_reason` is a small, closed vocabulary per domain, so failures
can be counted, grouped and alerted on.

## What go-see is not

It is not a replacement for all logging. Diagnostic narration, the kind you read while
debugging and throw away afterwards, still belongs to your logger, and it shares the
same zap pipeline. Events are the durable tier: occurrences relevant enough to be
queried, alerted on, or reported against.

It is not a metrics or tracing library either. go-see calls your `Measure` and stays
out of the way, so any metrics backend works.

## Prior art

The idea is not new. Stripe's [canonical log lines](https://stripe.com/blog/canonical-log-lines),
wide events, and OpenTelemetry's [event names](https://opentelemetry.io/docs/specs/semconv/general/events/)
all push logs towards stable, structured occurrences. go-see's take is to make the
event a Go type: its id is validated at startup, its payload is checked by the
compiler, and every projection lives on the type itself.
