import subprocess
import signal
import sys
import os
import asyncio
from typing import AsyncGenerator, Optional

import httpx
import websockets
from fastapi import FastAPI, Request, WebSocket, WebSocketDisconnect
from fastapi.responses import Response
from contextlib import asynccontextmanager

GO_BINARY: str = os.path.join(os.path.dirname(__file__), "quizhub")
GO_PORT: str = os.getenv("GO_PORT", "8002")
GO_URL: str = f"http://127.0.0.1:{GO_PORT}"
GO_WS_URL: str = f"ws://127.0.0.1:{GO_PORT}"

go_process: Optional[subprocess.Popen] = None


@asynccontextmanager
async def lifespan(application: FastAPI) -> AsyncGenerator[None, None]:
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


@app.websocket("/api/ws")
async def websocket_proxy(ws: WebSocket) -> None:
    await ws.accept()

    params: dict[str, str] = dict(ws.query_params)
    qs: str = "&".join(f"{k}={v}" for k, v in params.items())
    go_ws_url: str = f"{GO_WS_URL}/api/ws?{qs}" if qs else f"{GO_WS_URL}/api/ws"

    try:
        async with websockets.connect(go_ws_url) as go_ws:
            async def client_to_go() -> None:
                try:
                    while True:
                        data: str = await ws.receive_text()
                        await go_ws.send(data)
                except WebSocketDisconnect:
                    pass
                except Exception:
                    pass

            async def go_to_client() -> None:
                try:
                    async for msg in go_ws:
                        await ws.send_text(msg)
                except Exception:
                    pass

            done, pending = await asyncio.wait(
                [asyncio.create_task(client_to_go()), asyncio.create_task(go_to_client())],
                return_when=asyncio.FIRST_COMPLETED,
            )
            for task in pending:
                task.cancel()
    except Exception:
        pass
    finally:
        try:
            await ws.close()
        except Exception:
            pass


@app.api_route("/api/{path:path}", methods=["GET", "POST", "PUT", "DELETE", "OPTIONS"])
async def proxy_api(request: Request, path: str) -> Response:
    url: str = f"/api/{path}"
    body: bytes = await request.body()
    headers: dict[str, str] = dict(request.headers)
    headers.pop("host", None)

    resp: httpx.Response = await client.request(
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
async def proxy_static(request: Request, path: str = "") -> Response:
    url: str = f"/{path}" if path else "/"
    resp: httpx.Response = await client.get(url)
    return Response(
        content=resp.content,
        status_code=resp.status_code,
        headers=dict(resp.headers),
    )
