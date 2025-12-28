# video-subtitle

Generate subtitle files from a video or audio file using OpenAI Whisper, then translate the text.

## Requirements

- Go 1.22+
- ffmpeg on PATH
- ffprobe on PATH (bundled with ffmpeg; required for chunking)
- `OPENAI_API_KEY` environment variable

## Install

```bash
make build
```

For a GOPATH install:

```bash
make install
```

The `make build` target places the binary at `video-subtitle/bin/video-subtitle`.

## Usage

```bash
video-subtitle /path/to/video.mp4
```

Defaults:

- Whisper model: `whisper-1`
- Source language: `ja`
- Target language: `zh-TW`
- Output: same path with `.srt`
- Progress output: enabled

## Options

```bash
video-subtitle /path/to/video.mp4 \
  --output /path/to/output.srt \
  --source-lang ja \
  --target-lang zh-TW \
  --whisper-model whisper-1 \
  --translate-model gpt-4o-mini
```

To only generate a transcript without translation:

```bash
video-subtitle /path/to/video.mp4 --no-translate
```

For large inputs (auto-chunking kicks in by size, or you can force it):

```bash
video-subtitle /path/to/video.mp4 --chunk-seconds 600
```

Auto-chunk threshold (in MB) is configurable:

```bash
video-subtitle /path/to/video.mp4 --max-audio-mb 20
```

To silence progress output:

```bash
video-subtitle /path/to/video.mp4 --quiet
```

To speed up translation with concurrency:

```bash
video-subtitle /path/to/video.mp4 --translate-workers 6
```

To skip translation for short, low-info segments:

```bash
video-subtitle /path/to/video.mp4 --min-translate-chars 4
```

To extend the OpenAI request timeout:

```bash
video-subtitle /path/to/video.mp4 --timeout-seconds 1200
```
