#!/usr/bin/env python3
"""
Audio & video transcription helper script for FrontPocket.
Uses WhisperX (or Whisper) with GPU acceleration to transcribe audio/video files to structured JSON.
"""

import argparse
import json
import os
import sys

def find_ffmpeg():
    import shutil
    p = shutil.which("ffmpeg")
    if p:
        return p
    p = os.path.expanduser("~/.local/bin/ffmpeg")
    if os.path.isfile(p) and os.access(p, os.X_OK):
        return p
    p = "/home/dev/.openclaw/workspace/Transcripto/ffmpeg-7.0.2-amd64-static/ffmpeg"
    if os.path.isfile(p) and os.access(p, os.X_OK):
        return p
    return None

def transcribe(input_file, output_file, model_name="base", device=None, language="en"):
    ffmpeg_bin = find_ffmpeg()
    if ffmpeg_bin:
        ffmpeg_dir = os.path.dirname(ffmpeg_bin)
        if ffmpeg_dir not in os.environ.get("PATH", ""):
            os.environ["PATH"] = ffmpeg_dir + ":" + os.environ.get("PATH", "")

    if not os.path.isfile(input_file):
        print(f"Error: input file '{input_file}' not found", file=sys.stderr)
        sys.exit(1)

    import torch
    if device is None:
        device = "cuda" if torch.cuda.is_available() else "cpu"

    compute_type = "float16" if device == "cuda" else "int8"

    try:
        import whisperx
        model = whisperx.load_model(model_name, device, compute_type=compute_type, language=language)
        audio = whisperx.load_audio(input_file)
        result = model.transcribe(audio, batch_size=16)

        segments = []
        for s in result.get("segments", []):
            text = s.get("text", "").strip()
            if text:
                segments.append({
                    "start": round(s.get("start", 0.0), 2),
                    "end": round(s.get("end", 0.0), 2),
                    "text": text
                })

        duration = 0.0
        if segments:
            duration = segments[-1]["end"]

        out_data = {
            "file": os.path.abspath(input_file),
            "engine": "whisperx",
            "model": model_name,
            "device": device,
            "duration": round(duration, 2),
            "segments": segments
        }
    except Exception as e_wx:
        try:
            import whisper
            model = whisper.load_model(model_name, device=device)
            result = model.transcribe(input_file, language=language)
            segments = []
            for s in result.get("segments", []):
                text = s.get("text", "").strip()
                if text:
                    segments.append({
                        "start": round(s.get("start", 0.0), 2),
                        "end": round(s.get("end", 0.0), 2),
                        "text": text
                    })
            duration = 0.0
            if segments:
                duration = segments[-1]["end"]
            out_data = {
                "file": os.path.abspath(input_file),
                "engine": "whisper",
                "model": model_name,
                "device": device,
                "duration": round(duration, 2),
                "segments": segments
            }
        except Exception as e_w:
            print(f"Error transcribing with whisperx ({e_wx}) and whisper ({e_w})", file=sys.stderr)
            sys.exit(2)

    os.makedirs(os.path.dirname(os.path.abspath(output_file)), exist_ok=True)
    with open(output_file, "w", encoding="utf-8") as f:
        json.dump(out_data, f, indent=2, ensure_ascii=False)

    print(f"Transcription complete: {len(out_data['segments'])} segments written to {output_file}")

def main():
    parser = argparse.ArgumentParser(description="Transcribe audio or video file with Whisper")
    parser.add_argument("--input", "-i", required=True, help="Input audio or video file")
    parser.add_argument("--output", "-o", required=True, help="Output JSON transcript path")
    parser.add_argument("--model", "-m", default="base", help="Whisper model (base, small, medium, large-v3)")
    parser.add_argument("--device", "-d", default=None, help="Device (cuda or cpu)")
    parser.add_argument("--language", "-l", default="en", help="Audio language code (e.g. en)")

    args = parser.parse_args()
    transcribe(args.input, args.output, args.model, args.device, args.language)

if __name__ == "__main__":
    main()
