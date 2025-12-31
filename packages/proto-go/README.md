# proto-go

Shared Protocol Buffer generated code for Continua.

## Status

**Internal use only.** This module is not published to a registry.

Both `server` and `apps/tui` depend on this module via the root `go.work` file.

## Versioning

Currently unversioned (v0.0.0). When we need to:
1. Support external consumers, OR
2. Decouple release cycles

We will:
1. Tag releases as `packages/proto-go/v0.1.0`
2. Update consumers to use real versions

Until then, all changes are committed together in the monorepo.

## Usage

```go
import (
    apiv1 "github.com/continua-ai/continua/packages/proto-go/continua/api/v1"
    eventsv1 "github.com/continua-ai/continua/packages/proto-go/continua/events/v1"
)
```
