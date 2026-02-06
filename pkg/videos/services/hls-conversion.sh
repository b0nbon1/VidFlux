#!/bin/bash

INPUT_FILE="$1"
OUTPUT_DIR="$2"
MODE="${3:-all}"

if [ -z "$INPUT_FILE" ] || [ -z "$OUTPUT_DIR" ]; then
    echo "Usage: $0 <input_file> <output_dir> [all|video|audio]"
    exit 1
fi

mkdir -p "$OUTPUT_DIR"

SOURCE_HEIGHT=$(ffprobe -v error -select_streams v:0 -show_entries stream=height -of csv=p=0 "$INPUT_FILE")
SOURCE_WIDTH=$(ffprobe -v error -select_streams v:0 -show_entries stream=width -of csv=p=0 "$INPUT_FILE")

echo "Source resolution: ${SOURCE_WIDTH}x${SOURCE_HEIGHT}"

HLS_TIME=4
HLS_LIST_SIZE=0
HLS_FLAGS="independent_segments"
HLS_PLAYLIST_TYPE="vod"

X264_PRESET="slow"
FPS=24
GOP_SIZE=$((FPS * HLS_TIME))
KEYFRAME_EXPR="expr:gte(t,n_forced*$HLS_TIME)"

COMMON_VIDEO_FLAGS="-preset $X264_PRESET -profile:v high -level 4.1 -pix_fmt yuv420p -sc_threshold 0 -r $FPS -g $GOP_SIZE -keyint_min $GOP_SIZE -force_key_frames $KEYFRAME_EXPR"

generate_video_streams() {
    local filters=""
    local maps=""
    local outputs=""
    local stream_idx=0
    local var_map=""

    add_stream() {
        local scale="$1"
        local bitrate="$2"
        local maxrate="$3"
        local bufsize="$4"
        local name="$5"

        filters+="[0:v]scale=$scale:force_original_aspect_ratio=decrease,pad=$scale:(ow-iw)/2:(oh-ih)/2[v$stream_idx];"
        maps+=" -map [v$stream_idx]"
        outputs+=" -c:v:$stream_idx libx264 -b:v:$stream_idx $bitrate -maxrate:v:$stream_idx $maxrate -bufsize:v:$stream_idx $bufsize"
        var_map+="v:$stream_idx,name:$name "
        stream_idx=$((stream_idx + 1))
    }

    add_stream "640:360" "800k" "900k" "1200k" "360p"

    if [ "$SOURCE_HEIGHT" -ge 480 ]; then
        add_stream "854:480" "1400k" "1600k" "2100k" "480p"
    fi

    if [ "$SOURCE_HEIGHT" -ge 720 ]; then
        add_stream "1280:720" "2800k" "3200k" "4200k" "720p"
    fi

    if [ "$SOURCE_HEIGHT" -ge 1080 ]; then
        add_stream "1920:1080" "5000k" "5800k" "7500k" "1080p"
    fi

    filters=$(echo "$filters" | sed 's/;$//')

    echo "Generating $stream_idx video streams..."

    ffmpeg -y -i "$INPUT_FILE" \
        -filter_complex "$filters" \
        $maps \
        $outputs \
        -an \
        $COMMON_VIDEO_FLAGS \
        -vsync cfr \
        -avoid_negative_ts make_zero \
        -f hls \
        -hls_time "$HLS_TIME" \
        -hls_list_size "$HLS_LIST_SIZE" \
        -hls_playlist_type "$HLS_PLAYLIST_TYPE" \
        -hls_flags "$HLS_FLAGS" \
        -hls_segment_filename "$OUTPUT_DIR/video_%v_%03d.ts" \
        -master_pl_name "video_master.m3u8" \
        -var_stream_map "$var_map" \
        "$OUTPUT_DIR/video_%v.m3u8"
}

generate_audio_stream() {
    echo "Generating audio-only stream..."

    ffmpeg -y -i "$INPUT_FILE" \
        -vn \
        -c:a aac \
        -b:a 128k \
        -ac 2 \
        -ar 48000 \
        -af "aresample=async=1" \
        -f hls \
        -hls_time "$HLS_TIME" \
        -hls_list_size "$HLS_LIST_SIZE" \
        -hls_playlist_type "$HLS_PLAYLIST_TYPE" \
        -hls_flags "$HLS_FLAGS" \
        -hls_segment_filename "$OUTPUT_DIR/audio_%03d.ts" \
        "$OUTPUT_DIR/audio.m3u8"
}

generate_combined_streams() {
    local filters=""
    local maps=""
    local outputs=""
    local stream_idx=0
    local var_map=""

    add_stream() {
        local scale="$1"
        local bitrate="$2"
        local maxrate="$3"
        local bufsize="$4"
        local abitrate="$5"
        local name="$6"

        filters+="[0:v]scale=$scale:force_original_aspect_ratio=decrease,pad=$scale:(ow-iw)/2:(oh-ih)/2[v$stream_idx];"
        maps+=" -map [v$stream_idx] -map 0:a"
        outputs+=" -c:v:$stream_idx libx264 -b:v:$stream_idx $bitrate -maxrate:v:$stream_idx $maxrate -bufsize:v:$stream_idx $bufsize -c:a:$stream_idx aac -b:a:$stream_idx $abitrate"
        var_map+="v:$stream_idx,a:$stream_idx,name:$name "
        stream_idx=$((stream_idx + 1))
    }

    add_stream "640:360" "800k" "900k" "1200k" "96k" "360p"

    if [ "$SOURCE_HEIGHT" -ge 480 ]; then
        add_stream "854:480" "1400k" "1600k" "2100k" "128k" "480p"
    fi

    if [ "$SOURCE_HEIGHT" -ge 720 ]; then
        add_stream "1280:720" "2800k" "3200k" "4200k" "128k" "720p"
    fi

    if [ "$SOURCE_HEIGHT" -ge 1080 ]; then
        add_stream "1920:1080" "5000k" "5800k" "7500k" "192k" "1080p"
    fi

    filters=$(echo "$filters" | sed 's/;$//')

    echo "Generating $stream_idx combined streams..."

    ffmpeg -y -i "$INPUT_FILE" \
        -filter_complex "$filters" \
        $maps \
        $outputs \
        $COMMON_VIDEO_FLAGS \
        -af "aresample=async=1" \
        -vsync cfr \
        -avoid_negative_ts make_zero \
        -f hls \
        -hls_time "$HLS_TIME" \
        -hls_list_size "$HLS_LIST_SIZE" \
        -hls_playlist_type "$HLS_PLAYLIST_TYPE" \
        -hls_flags "$HLS_FLAGS" \
        -hls_segment_filename "$OUTPUT_DIR/combined_%v_%03d.ts" \
        -master_pl_name "master.m3u8" \
        -var_stream_map "$var_map" \
        "$OUTPUT_DIR/combined_%v.m3u8"
}

case "$MODE" in
    video) generate_video_streams ;;
    audio) generate_audio_stream ;;
    all)
        generate_video_streams
        generate_audio_stream
        generate_combined_streams
        ;;
    *) echo "Invalid mode: $MODE"; exit 1 ;;
esac

echo "HLS output ready: $OUTPUT_DIR"
