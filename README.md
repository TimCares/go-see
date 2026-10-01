# go-see

<div align="center">

<img src="assets/logo.png" alt="go-see logo" width="260">

**Typed, semantic events instead of free form log lines.**

[![Go Reference](https://pkg.go.dev/badge/github.com/TimCares/go-see.svg)](https://pkg.go.dev/github.com/TimCares/go-see)
[![CI](https://github.com/TimCares/go-see/actions/workflows/ci.yml/badge.svg)](https://github.com/TimCares/go-see/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/TimCares/go-see)](https://goreportcard.com/report/github.com/TimCares/go-see)
[![Release](https://img.shields.io/github/v/release/TimCares/go-see?sort=semver)](https://github.com/TimCares/go-see/releases)
[![semantic-release: conventionalcommits](https://img.shields.io/badge/semantic--release-conventionalcommits-e10079?logo=semantic-release)](https://github.com/semantic-release/semantic-release)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

</div>

"Upload failed", "failed to upload", "could not upload artifact" are three log lines
for one occurrence, and none of them is safe to alert on. go-see replaces them with
**events**: a stable, dot separated id plus a typed payload, recorded through a single
`Emit`. Each event is turned into a structured [zap](https://github.com/uber-go/zap)
record and, optionally, into metrics, so the log and the metrics can never tell
different stories.

```go
see.Emit(ctx, ArtifactUpload{
	URI:   uri,
	Error: see.Fail(err, UploadNotStarted),
})
```

Why this is worth it is laid out in [docs/motivation.md](docs/motivation.md).

## Install

```sh
go get github.com/TimCares/go-see
```

The package name is `see`. It requires Go 1.24 or newer.

## Quick start

**1. Define an event**, once, next to the code it describes:

```go
var ArtifactUploadID = see.NewID("storage.artifact.upload")

type UploadReason string

const (
	UploadTimeout    UploadReason = "timeout"
	UploadNotStarted UploadReason = "not_started"
)

// ArtifactUpload records that one artifact was persisted to object storage, or failed to be.
type ArtifactUpload struct {
	URI   string                   `json:"uri"`
	Error *see.Error[UploadReason] `json:"error"` // No error means success.
}

func (ArtifactUpload) ID() see.ID { return ArtifactUploadID }
```

**2. Configure an emitter**, once, at startup:

```go
logger, _ := zap.NewProduction()

emitter, err := see.New(see.Config{Logger: logger})
if err != nil {
	return err
}
see.SetDefault(emitter)
```

**3. Emit**, wherever the occurrence happens:

```go
see.Emit(ctx, ArtifactUpload{URI: uri})
```

Which is recorded as:

```json
{
  "level": "info",
  "msg": "storage.artifact.upload",
  "event": {
    "id": "storage.artifact.upload",
    "data": { "uri": "s3://calls/abc/recording.ogg", "error": null }
  }
}
```

## Concepts

**Ids** are the vocabulary. `see.NewID` validates them and panics at startup if an id
is malformed, registered twice, or a prefix of another id. An id is at least two
lowercase segments, e.g. `storage.artifact.upload`. Ids are a stable contract:
dashboards and alerts depend on them, so never repurpose or rename one.

**Payloads** are the event's exported fields, encoded through their `json` tags.
Events on a hot path can implement `zapcore.ObjectMarshaler` to skip reflection.

**Levels** are a property of the occurrence, not of the call site. Implement `Level`
to deviate from info:

```go
// Level reports a failed upload as a warning: the call itself still succeeded.
func (e ArtifactUpload) Level() see.Level {
	if e.Error != nil {
		return see.LevelWarn
	}
	return see.LevelInfo
}
```

**Metrics** are opted into by implementing `Measure`. go-see does not care which
metrics library you use, it only calls `Measure` after the record is written and
recovers a panic, so a broken label set degrades the metric rather than the event:

```go
// Measure counts the attempt, and separately counts it if it failed.
func (e ArtifactUpload) Measure(ctx context.Context) {
	uploadsTotal.Inc()
	if e.Error != nil {
		uploadErrorsTotal.WithLabelValues(string(e.Error.Reason)).Inc()
	}
}
```

**Errors** pair the raw `error` with a stable reason. The raw error keeps every detail,
the reason is what you filter and alert on. Both are recorded as `raw_error` and
`stable_error_reason`.

**Context** carries ambient fields, e.g. a session id, so they do not have to be
repeated in every payload:

```go
ctx = see.With(ctx, zap.String("session_id", sessionID))
```

## Reserved ids

The `see` root belongs to go-see and is used for meta observability:

| Id                  | Level | Meaning                                              |
| ------------------- | ----- | ---------------------------------------------------- |
| `see.emit.error`    | error | An event without a valid id was emitted.             |
| `see.measure.error` | warn  | An event's `Measure` panicked, its metrics are lost. |

## Status

go-see is pre `v1`. The API may still change between minor versions until then.
Every release and its notes are listed on the [releases page](https://github.com/TimCares/go-see/releases).

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md).

## License

[MIT](LICENSE)
