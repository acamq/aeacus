#!/usr/bin/env python3
# /// script
# requires-python = ">=3.12"
# dependencies = []
# ///
# ─── How to run ───
# uv run misc/tests/freebsd/serve-image.py IMAGE PORT_FILE PID_FILE

from __future__ import annotations

import http.server
import os
import sys
from dataclasses import dataclass
from pathlib import Path
from typing import ClassVar, Final, override
from urllib.parse import unquote, urlsplit

CHUNK_SIZE: Final = 1024 * 1024


@dataclass(frozen=True, slots=True)
class ServerError(Exception):
    detail: str

    def __str__(self) -> str:
        return self.detail


class ExactImageHandler(http.server.BaseHTTPRequestHandler):
    image: ClassVar[Path]

    def do_HEAD(self) -> None:
        self.send_image(include_body=False)

    def do_GET(self) -> None:
        self.send_image(include_body=True)

    def send_image(self, *, include_body: bool) -> None:
        request_path = unquote(urlsplit(self.path).path)
        if request_path != f"/{self.image.name}":
            self.send_error(http.HTTPStatus.NOT_FOUND)
            return
        size = self.image.stat().st_size
        self.send_response(http.HTTPStatus.OK)
        self.send_header("Content-Type", "application/octet-stream")
        self.send_header("Content-Length", str(size))
        self.send_header("Cache-Control", "no-store")
        self.end_headers()
        if include_body:
            with self.image.open("rb") as source:
                while chunk := source.read(CHUNK_SIZE):
                    self.wfile.write(chunk)

    @override
    def log_message(self, format: str, *arguments: object) -> None:
        print(f"{self.client_address[0]} {format % arguments}", flush=True)


def write_exclusive(path: Path, value: str) -> None:
    descriptor = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    with os.fdopen(descriptor, "w", encoding="ascii") as output:
        output.write(f"{value}\n")
        output.flush()
        os.fsync(output.fileno())


def main(arguments: list[str]) -> int:
    if len(arguments) != 3:
        raise ServerError(detail="usage: serve-image.py IMAGE PORT_FILE PID_FILE")
    image, port_file, pid_file = map(Path, arguments)
    if not image.is_file():
        raise ServerError(detail=f"image is not a regular file: {image}")
    ExactImageHandler.image = image.resolve()
    server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), ExactImageHandler)
    try:
        write_exclusive(pid_file, str(os.getpid()))
        write_exclusive(port_file, str(server.server_port))
        server.serve_forever(poll_interval=0.25)
    finally:
        server.server_close()
    return 0


if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1:]))
