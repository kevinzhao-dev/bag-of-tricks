# video-subtitle

Generate subtitle files from a video or audio file using OpenAI Whisper, then translate the text.

## Requirements

- Python 3.9+
- ffmpeg on PATH
- ffprobe on PATH (bundled with ffmpeg; required for chunking)
- `OPENAI_API_KEY` environment variable

## Install

```bash
pip install -e .
```

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
