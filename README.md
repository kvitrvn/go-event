# go-evt

`go-evt` is a lightweight event emitter for Go applications that need predictable listener ordering and strong observability.

## Upgrades included

- Listener ordering by priority (lowest number executed first).
- Context-aware event execution (`EmitWithContext`).
- Rich execution metrics with `EmitDetailedWithContext`.
- Concurrent dispatch with worker pool using `EmitAsyncWithContext`.
- Optional context-aware listeners via `ContextListener`.
- Aggregated errors with event and listener metadata.

## Quick start

```go
emitter := event.NewEmitter()
emitter.AddListener(NewUserListener("user.general", 10))

result, err := emitter.EmitDetailedWithContext(context.Background(), "user.general", user)
if err != nil {
    log.Printf("emit failed: %v", err)
}
log.Printf("listeners started=%d success=%d failed=%d", result.Started, result.Successful, result.Failed)
```

## API overview

### Sequential (ordered)

Use when listeners have strong ordering dependencies:

```go
result, err := emitter.EmitDetailedWithContext(ctx, "event.type", payload)
```

### Concurrent (worker pool)

Use when listeners are independent and mostly I/O bound:

```go
result, err := emitter.EmitAsyncWithContext(ctx, "event.type", payload, 8)
```

If `workers <= 0`, the emitter uses `GOMAXPROCS`.

## GitHub Actions

The repository includes two workflows:

- `ci.yml`: format check, vet, tests, race detector, and module tidiness checks.
- `security.yml`: CodeQL analysis for Go.
