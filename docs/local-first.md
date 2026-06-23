# Local-First Operation

FrontPocket is designed to work fully on local infrastructure.

## Defaults

```env
EMBEDDING_PROVIDER=ollama
QDRANT_URL=http://qdrant:6333
REDIS_URL=redis://redis:6379/0
```

## Dependency bootstrap

First, run the root build helper:

```bash
./make_all.sh
```

If Qdrant or Redis are missing, run:

```bash
./scripts/install_qdrant_redis.sh
```

The installer checks what is already available and only starts missing services.

Cloud embedding providers remain optional and explicitly configured.
