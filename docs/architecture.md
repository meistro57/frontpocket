# FrontPocket Architecture

```text
Client / Agent / Chat Bridge
        ↓
FrontPocket Go HTTP API
        ↓
Qdrant (long-term semantic memory) + Redis (working/session memory)
```

The API is the only public boundary. Qdrant and Redis should stay private in non-local deployments.

Current API version: `0.1.0`
