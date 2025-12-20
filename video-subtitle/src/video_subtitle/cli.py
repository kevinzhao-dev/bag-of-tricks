import argparse
import os
import random
import shutil
import subprocess
import sys
import tempfile
import time
from pathlib import Path

import openai
from openai import OpenAI


DEFAULT_CHUNK_SECONDS = 600
DEFAULT_MAX_AUDIO_MB = 24


def format_srt_timestamp(seconds):
    millis = int(round(seconds * 1000))
    hours, remainder = divmod(millis, 3600 * 1000)
    minutes, remainder = divmod(remainder, 60 * 1000)
    secs, millis = divmod(remainder, 1000)
    return f"{hours:02d}:{minutes:02d}:{secs:02d},{millis:03d}"


def write_srt(segments, output_path):
    lines = []
    for idx, segment in enumerate(segments, start=1):
        start_ts = format_srt_timestamp(segment["start"])
        end_ts = format_srt_timestamp(segment["end"])
        text = segment["text"].strip()
        lines.append(str(idx))
        lines.append(f"{start_ts} --> {end_ts}")
        lines.append(text)
        lines.append("")
    output_path.write_text("\n".join(lines), encoding="utf-8")


def ensure_openai_key():
    if not os.getenv("OPENAI_API_KEY"):
        raise RuntimeError("OPENAI_API_KEY is not set")


def run_ffmpeg_extract(input_path, output_path):
    cmd = [
        "ffmpeg",
        "-y",
        "-i",
        str(input_path),
        "-vn",
        "-ac",
        "1",
        "-ar",
        "16000",
        "-f",
        "wav",
        str(output_path),
    ]
    result = subprocess.run(cmd, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
    if result.returncode != 0:
        raise RuntimeError(f"ffmpeg failed: {result.stderr.strip()}")


def run_ffmpeg_extract_segment(input_path, output_path, start_seconds, duration_seconds):
    cmd = [
        "ffmpeg",
        "-y",
        "-ss",
        f"{start_seconds:.3f}",
        "-t",
        f"{duration_seconds:.3f}",
        "-i",
        str(input_path),
        "-vn",
        "-ac",
        "1",
        "-ar",
        "16000",
        "-f",
        "wav",
        str(output_path),
    ]
    result = subprocess.run(cmd, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
    if result.returncode != 0:
        raise RuntimeError(f"ffmpeg failed: {result.stderr.strip()}")


def get_audio_duration(audio_path):
    cmd = [
        "ffprobe",
        "-v",
        "error",
        "-show_entries",
        "format=duration",
        "-of",
        "default=noprint_wrappers=1:nokey=1",
        str(audio_path),
    ]
    result = subprocess.run(cmd, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
    if result.returncode != 0:
        raise RuntimeError(f"ffprobe failed: {result.stderr.strip()}")
    try:
        return float(result.stdout.strip())
    except ValueError as exc:
        raise RuntimeError("Failed to parse audio duration.") from exc


def get_audio_size(audio_path):
    try:
        return audio_path.stat().st_size
    except OSError as exc:
        raise RuntimeError(f"Failed to read extracted audio: {exc}") from exc


def format_megabytes(num_bytes):
    return num_bytes / (1024 * 1024)


def describe_error(exc):
    status = getattr(exc, "status_code", None)
    if status:
        return f"{exc.__class__.__name__} ({status})"
    return exc.__class__.__name__


def retry(
    func,
    max_retries=3,
    base_delay=1.0,
    max_delay=20.0,
    should_retry=None,
    on_retry=None,
):
    for attempt in range(max_retries):
        try:
            return func()
        except Exception as exc:
            if isinstance(exc, KeyboardInterrupt):
                raise
            if attempt >= max_retries - 1:
                raise
            if should_retry is not None and not should_retry(exc):
                raise
            delay = min(max_delay, base_delay * (2 ** attempt))
            delay += random.uniform(0, delay * 0.25)
            if on_retry is not None:
                on_retry(attempt + 1, delay, exc)
            time.sleep(delay)


def is_retryable_error(exc):
    if isinstance(
        exc,
        (
            openai.RateLimitError,
            openai.APIConnectionError,
            openai.APITimeoutError,
            openai.InternalServerError,
        ),
    ):
        return True
    status = getattr(exc, "status_code", None)
    return status in {408, 409, 429, 500, 502, 503, 504}


def transcribe_audio(client, audio_path, whisper_model, source_lang, on_retry=None):
    def _call():
        with open(audio_path, "rb") as audio_file:
            return client.audio.transcriptions.create(
                model=whisper_model,
                file=audio_file,
                response_format="verbose_json",
                language=source_lang,
                timestamp_granularities=["segment"],
            )

    response = retry(
        _call,
        max_retries=4,
        base_delay=1.0,
        should_retry=is_retryable_error,
        on_retry=on_retry,
    )
    segments = []
    for segment in response.segments or []:
        segments.append(
            {
                "start": float(segment.start),
                "end": float(segment.end),
                "text": segment.text or "",
            }
        )
    if not segments and getattr(response, "text", None):
        segments.append({"start": 0.0, "end": 0.0, "text": response.text})
    return segments


def transcribe_audio_in_chunks(
    client,
    audio_path,
    whisper_model,
    source_lang,
    chunk_seconds,
    verbose=False,
    on_retry=None,
):
    duration = get_audio_duration(audio_path)
    if duration <= 0:
        raise RuntimeError("Audio duration is zero.")

    segments = []
    current = 0.0
    chunk_index = 0
    audio_dir = Path(audio_path).parent

    while current < duration - 0.01:
        remaining = duration - current
        segment_duration = min(chunk_seconds, remaining)
        chunk_path = audio_dir / f"chunk_{chunk_index:04d}.wav"
        if verbose:
            print(
                f"Transcribing chunk {chunk_index + 1} at {current:.1f}s...",
                file=sys.stderr,
            )
        run_ffmpeg_extract_segment(audio_path, chunk_path, current, segment_duration)
        chunk_segments = transcribe_audio(
            client,
            chunk_path,
            whisper_model,
            source_lang,
            on_retry=on_retry,
        )
        for segment in chunk_segments:
            segment["start"] += current
            segment["end"] += current
        segments.extend(chunk_segments)
        current += segment_duration
        chunk_index += 1

    return segments


def should_fallback_to_chunking(exc):
    if isinstance(exc, openai.BadRequestError):
        message = str(exc).lower()
        return "reading your request" in message or "invalid_request_error" in message
    status = getattr(exc, "status_code", None)
    return status in {413, 429, 500, 502, 503, 504}


def choose_chunk_seconds(audio_path, default_chunk_seconds, max_audio_bytes):
    duration = get_audio_duration(audio_path)
    if duration <= 0:
        return default_chunk_seconds
    size_bytes = get_audio_size(audio_path)
    if size_bytes <= 0:
        return default_chunk_seconds
    bytes_per_second = size_bytes / duration
    if bytes_per_second <= 0:
        return default_chunk_seconds
    estimated = int(max_audio_bytes / bytes_per_second)
    if estimated <= 0:
        return default_chunk_seconds
    return max(30, min(default_chunk_seconds, estimated))
    return False


def translate_segment(client, text, source_lang, target_lang, model, on_retry=None):
    cleaned = text.strip()
    if not cleaned:
        return text

    system_prompt = "You are a precise translator. Return only the translation."
    user_prompt = (
        f"Translate the following text from {source_lang} to {target_lang}. "
        "Preserve punctuation and line breaks."
    )

    def _call():
        response = client.chat.completions.create(
            model=model,
            messages=[
                {"role": "system", "content": system_prompt},
                {"role": "user", "content": f"{user_prompt}\n\n{cleaned}"},
            ],
            temperature=0,
        )
        return response.choices[0].message.content.strip()

    return retry(
        _call,
        max_retries=4,
        base_delay=1.0,
        should_retry=is_retryable_error,
        on_retry=on_retry,
    )


def translate_segments(
    client, segments, source_lang, target_lang, model, verbose=False, on_retry=None
):
    translated = []
    for segment in segments:
        text = segment["text"]
        if verbose:
            print("Translating segment...", file=sys.stderr)
        translated_text = translate_segment(
            client, text, source_lang, target_lang, model, on_retry=on_retry
        )
        translated.append({**segment, "text": translated_text})
    return translated


def build_arg_parser():
    parser = argparse.ArgumentParser(
        description="Generate SRT subtitles from audio/video using Whisper and translate them."
    )
    parser.add_argument("input", help="Path to a video or audio file")
    parser.add_argument(
        "--output",
        "-o",
        help="Output SRT path (defaults to input path with .srt)",
    )
    parser.add_argument("--source-lang", default="ja", help="Source language")
    parser.add_argument("--target-lang", default="zh-TW", help="Target language")
    parser.add_argument("--whisper-model", default="whisper-1", help="Whisper model")
    parser.add_argument(
        "--translate-model",
        default="gpt-4o-mini",
        help="Translation model",
    )
    parser.add_argument(
        "--no-translate",
        action="store_true",
        help="Skip translation and output original transcript",
    )
    parser.add_argument(
        "--chunk-seconds",
        type=int,
        default=0,
        help="Split audio into chunks of N seconds before transcription",
    )
    parser.add_argument(
        "--max-audio-mb",
        type=int,
        default=DEFAULT_MAX_AUDIO_MB,
        help="Auto-chunk when extracted audio exceeds this size (MB)",
    )
    parser.add_argument(
        "--keep-audio",
        action="store_true",
        help="Keep the extracted audio file",
    )
    parser.add_argument(
        "--quiet",
        action="store_true",
        help="Suppress progress output",
    )
    return parser


def main():
    parser = build_arg_parser()
    args = parser.parse_args()

    input_path = Path(args.input).expanduser().resolve()
    if not input_path.exists():
        print(f"Input file not found: {input_path}", file=sys.stderr)
        return 1

    if shutil.which("ffmpeg") is None:
        print("ffmpeg is required on PATH.", file=sys.stderr)
        return 1

    try:
        ensure_openai_key()
    except RuntimeError as exc:
        print(str(exc), file=sys.stderr)
        return 1

    output_path = (
        Path(args.output).expanduser().resolve()
        if args.output
        else input_path.with_suffix(".srt")
    )

    client = OpenAI()

    def log(message):
        if not args.quiet:
            print(message, file=sys.stderr)

    def on_retry(action):
        def _handler(attempt, delay, exc):
            log(
                f"{action} failed; retrying in {delay:.1f}s "
                f"(attempt {attempt}). {describe_error(exc)}"
            )

        return _handler

    with tempfile.TemporaryDirectory() as tmp_dir:
        audio_path = Path(tmp_dir) / "audio.wav"
        log("Extracting audio...")
        try:
            run_ffmpeg_extract(input_path, audio_path)
        except RuntimeError as exc:
            print(str(exc), file=sys.stderr)
            return 1
        try:
            audio_size = get_audio_size(audio_path)
            if audio_size < 1024:
                print("Extracted audio is empty or too small.", file=sys.stderr)
                return 1
        except RuntimeError as exc:
            print(str(exc), file=sys.stderr)
            return 1

        log("Transcribing with Whisper...")
        max_audio_bytes = args.max_audio_mb * 1024 * 1024
        use_chunking = args.chunk_seconds > 0 or audio_size > max_audio_bytes
        chunk_seconds = args.chunk_seconds
        if use_chunking and shutil.which("ffprobe") is None:
            print("ffprobe is required for chunked transcription.", file=sys.stderr)
            return 1
        if args.chunk_seconds <= 0 and audio_size > max_audio_bytes:
            chunk_seconds = choose_chunk_seconds(
                audio_path, DEFAULT_CHUNK_SECONDS, max_audio_bytes
            )
            log(
                "Audio is large "
                f"({format_megabytes(audio_size):.1f} MB); "
                f"auto-chunking with {chunk_seconds}s segments."
            )
        elif args.chunk_seconds > 0:
            log(f"Chunking audio into {chunk_seconds}s segments.")
        try:
            if use_chunking:
                segments = transcribe_audio_in_chunks(
                    client,
                    audio_path,
                    args.whisper_model,
                    args.source_lang,
                    chunk_seconds,
                    verbose=not args.quiet,
                    on_retry=on_retry("Transcription"),
                )
            else:
                segments = transcribe_audio(
                    client,
                    audio_path,
                    args.whisper_model,
                    args.source_lang,
                    on_retry=on_retry("Transcription"),
                )
        except Exception as exc:
            if args.chunk_seconds <= 0 and should_fallback_to_chunking(exc):
                fallback_seconds = DEFAULT_CHUNK_SECONDS
                if shutil.which("ffprobe") is None:
                    print("ffprobe is required for chunked transcription.", file=sys.stderr)
                    return 1
                log(
                    "Whisper request failed; retrying in chunks. "
                    f"Chunk size: {fallback_seconds}s."
                )
                try:
                    segments = transcribe_audio_in_chunks(
                        client,
                        audio_path,
                        args.whisper_model,
                        args.source_lang,
                        fallback_seconds,
                        verbose=not args.quiet,
                        on_retry=on_retry("Transcription"),
                    )
                except Exception as inner_exc:
                    print(f"Transcription failed: {inner_exc}", file=sys.stderr)
                    return 1
            else:
                print(f"Transcription failed: {exc}", file=sys.stderr)
                return 1

        if not args.no_translate and args.source_lang != args.target_lang:
            log("Translating segments...")
            try:
                segments = translate_segments(
                    client,
                    segments,
                    args.source_lang,
                    args.target_lang,
                    args.translate_model,
                    verbose=not args.quiet,
                    on_retry=on_retry("Translation"),
                )
            except Exception as exc:
                print(f"Translation failed: {exc}", file=sys.stderr)
                return 1

        log("Writing SRT...")
        write_srt(segments, output_path)

        if args.keep_audio:
            kept = input_path.with_suffix(".wav")
            kept.write_bytes(audio_path.read_bytes())
            log(f"Kept audio at {kept}")

    log(f"Wrote {output_path}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
