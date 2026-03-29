import subprocess
import signal
import sys
import os
import httpx
from fastapi import FastAPI, Request
from fastapi.responses import Response, HTMLResponse
from contextlib import asynccontextmanager

GO_BINARY = os.path.join(os.path.dirname(__file__), "quizhub")
GO_PORT = "8002"
GO_URL = f"http://127.0.0.1:{GO_PORT}"

go_process = None


@asynccontextmanager
async def lifespan(application):
    global go_process
    env = os.environ.copy()
    env["QUIZHUB_PORT"] = GO_PORT
    go_process = subprocess.Popen(
        [GO_BINARY],
        env=env,
        cwd=os.path.dirname(__file__),
        stdout=sys.stdout,
        stderr=sys.stderr,
    )
    yield
    if go_process:
        go_process.send_signal(signal.SIGTERM)
        go_process.wait(timeout=5)


app = FastAPI(lifespan=lifespan)
client = httpx.AsyncClient(base_url=GO_URL, timeout=30.0)


@app.api_route("/api/{path:path}", methods=["GET", "POST", "PUT", "DELETE", "OPTIONS"])
async def proxy_api(request: Request, path: str):
    url = f"/api/{path}"
    body = await request.body()
    headers = dict(request.headers)
    headers.pop("host", None)

    resp = await client.request(
        method=request.method,
        url=url,
        content=body,
        headers=headers,
    )

    return Response(
        content=resp.content,
        status_code=resp.status_code,
        headers=dict(resp.headers),
    )


@app.get("/{path:path}")
async def proxy_static(request: Request, path: str = ""):
    url = f"/{path}" if path else "/"
    resp = await client.get(url)
    return Response(
        content=resp.content,
        status_code=resp.status_code,
        headers=dict(resp.headers),
    )
