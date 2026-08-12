#!/bin/sh
set -eu

if [ -n "${IMAGE_PID:-}" ]; then
    kill "$IMAGE_PID" 2>/dev/null || true
    wait "$IMAGE_PID" 2>/dev/null || true
fi
if [ -n "${IMAGE_PATH:-}" ]; then
    rm -f "$IMAGE_PATH"
fi
