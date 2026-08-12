#!/bin/sh
set -eu

if [ "$#" -ne 3 ]; then
    echo "usage: stage-image.sh RELEASE ARCH STAGE" >&2
    exit 2
fi

release=$1
arch=$2
stage=$3
lock=.github/freebsd-images.lock
row=$(awk -F '\t' -v release="$release" -v arch="$arch" 'NR > 2 && $1 == release && $2 == arch { print }' "$lock")
[ "$(printf '%s\n' "$row" | awk 'NF { count++ } END { print count + 0 }')" -eq 1 ]
filename=$(printf '%s\n' "$row" | cut -f3)
url=$(printf '%s\n' "$row" | cut -f4)
bytes=$(printf '%s\n' "$row" | cut -f5)
sha256=$(printf '%s\n' "$row" | cut -f6)

mkdir -p "$stage"
image="$stage/$filename"
curl --fail --location --proto '=https' --tlsv1.2 --retry 3 --output "$image" "$url"
[ "$(stat -c %s "$image")" = "$bytes" ]
printf '%s  %s\n' "$sha256" "$image" | sha256sum --check --strict

port_file="$stage/server.port"
pid_file="$stage/server.pid"
log_file="$stage/server.log"
nohup python3 misc/tests/freebsd/serve-image.py "$image" "$port_file" "$pid_file" >"$log_file" 2>&1 &
launcher_pid=$!
for _ in $(seq 1 100); do
    [ -s "$port_file" ] && [ -s "$pid_file" ] && break
    kill -0 "$launcher_pid" 2>/dev/null || exit 1
    sleep 0.1
done
[ -s "$port_file" ] && [ -s "$pid_file" ]
[ "$(cat "$pid_file")" = "$launcher_pid" ]
port=$(cat "$port_file")
case "$port" in *[!0-9]*|'') exit 1 ;; esac
curl --fail --silent --show-error --head "http://127.0.0.1:$port/$filename" >/dev/null
printf 'IMAGE_PATH=%s\nIMAGE_URL=http://127.0.0.1:%s/%s\nIMAGE_PID=%s\nIMAGE_LOG=%s\n' "$image" "$port" "$filename" "$launcher_pid" "$log_file" >> "$GITHUB_ENV"
