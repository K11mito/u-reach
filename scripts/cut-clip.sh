#!/usr/bin/env bash
# Cut a clip for voice cloning: mono 192k mp3.
#   scripts/cut-clip.sh <in> <start> <end> <out.mp3>     e.g. scripts/cut-clip.sh data/voice/source.wav 0:43 2:25 data/voice/clips/a.mp3
set -euo pipefail
[ $# -eq 4 ] || { sed -n 2,3p "$0"; exit 1; }
mkdir -p "$(dirname "$4")"
ffmpeg -y -v error -ss "$2" -to "$3" -i "$1" -vn -ac 1 -c:a libmp3lame -b:a 192k "$4"
echo "$4: $(ffprobe -v error -show_entries format=duration -of csv=p=0 "$4")s"
