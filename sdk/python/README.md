# observr — Python SDK

Zero-config observability for Python services. One line to start tracing.

```python
import observr
observr.init(service="my-api")  # HTTP tracing + logs + dashboard. Done.
```

## Install

```bash
pip install observr
```

## Quickstart

**Flask**
```python
import observr
from flask import Flask

observr.init(service="my-api")
app = Flask(__name__)
```

**FastAPI**
```python
import observr
from fastapi import FastAPI

observr.init(service="my-api")
app = FastAPI()
```

**Manual spans**
```python
with observr.get_client().span("db.query", table="users") as span:
    rows = db.execute("SELECT ...")
    span.set_attribute("row_count", len(rows))
```

## Configuration

```python
observr.init(
    service="my-api",           # Service name shown in dashboard
    collector_url="http://localhost:7676",  # observrd collector URL
    auto_instrument=True,       # Auto-detect Flask / FastAPI
    log_level="DEBUG",          # Minimum log level to capture
)
```

## Links

- [GitHub](https://github.com/your-org/observr)
- [Dashboard & collector](https://github.com/your-org/observr/tree/main/server)
