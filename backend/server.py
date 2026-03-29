import subprocess
import signal
import sys
import os
import asyncio
import httpx
import websockets
from fastapi import FastAPI, Request, WebSocket, WebSocketDisconnect
from fastapi.responses import Response
from contextlib import asynccontextmanager

GO_BINARY = os.path.join(os.path.dirname(__file__), "quizhub")
GO_PORT = "8002"
GO_URL = f"http://127.0.0.1:{GO_PORT}"
GO_WS_URL = f"ws://127.0.0.1:{GO_PORT}"

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


@app.websocket("/api/ws")
async def websocket_proxy(ws: WebSocket):
    await ws.accept()

    # Build query string from original request
    params = dict(ws.query_params)
    qs = "&".join(f"{k}={v}" for k, v in params.items())
    go_ws_url = f"{GO_WS_URL}/api/ws?{qs}" if qs else f"{GO_WS_URL}/api/ws"

    try:
        async with websockets.connect(go_ws_url) as go_ws:
            async def client_to_go():
                try:
                    while True:
                        data = await ws.receive_text()
                        await go_ws.send(data)
                except WebSocketDisconnect:
                    pass
                except Exception:
                    pass

            async def go_to_client():
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
