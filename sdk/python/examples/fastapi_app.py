"""
FastAPI example — start observrd first, then run this file.

    $ observrd start
    $ uvicorn examples.fastapi_app:app --reload
    $ curl http://localhost:8000/users
"""

import logging

import observr
from fastapi import FastAPI

observr.init(service="fastapi-example")

logger = logging.getLogger(__name__)
app = FastAPI()


@app.get("/users")
async def get_users():
    logger.info("Fetching user list")
    return {"users": [{"id": 1, "name": "Alice"}, {"id": 2, "name": "Bob"}]}


@app.get("/slow")
async def slow_endpoint():
    import asyncio
    await asyncio.sleep(2)
    return {"result": "done after 2s"}


@app.get("/error")
async def trigger_error():
    raise ValueError("Simulated application error")
