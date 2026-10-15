#!/usr/bin/env bash
# Download a video's audio into data/voice/ as source.<ext> plus a lossless source.wav for cutting.
#   scripts/pull-audio.sh [youtube-url] [out-dir]
# If YouTube blocks the download, retry with: YTDLP_ARGS="--cookies-from-browser firefox" scripts/pull-audio.sh
set -euo pipefail
URL="${1:-https://www.youtube.com/watch?v=w5112--wekM}"
OUT="${2:-data/voice}"
YTDLP="$(command -v yt-dlp || echo "$HOME/.local/bin/yt-dlp")"
mkdir -p "$OUT"

JS=""; command -v node >/dev/null && JS="--js-runtimes node"
"$YTDLP" $JS ${YTDLP_ARGS:-} -f bestaudio --no-playlist -o "$OUT/source.%(ext)s" "$URL"
SRC="$(ls "$OUT"/source.* | grep -v '\.wav$' | head -1)"
ffmpeg -y -v error -i "$SRC" -vn -c:a pcm_s16le "$OUT/source.wav"
echo "saved $SRC and $OUT/source.wav ($(ffprobe -v error -show_entries format=duration -of csv=p=0 "$OUT/source.wav")s)"
