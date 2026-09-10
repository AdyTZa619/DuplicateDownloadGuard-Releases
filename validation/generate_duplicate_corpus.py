"""Deterministic encoded-media corpus; no network, private media or third-party Python packages.

Usage: python validation/generate_duplicate_corpus.py /absolute/output
Requires FFmpeg with libx264, libx265 and libwebp. Images/footage are synthetic,
not a representative accuracy dataset. All transformations use real codecs.
"""
import hashlib
import json
import math
from pathlib import Path
import random
import shutil
import subprocess
import sys

out = Path(sys.argv[1]).resolve()
out.mkdir(parents=True, exist_ok=True)
ff = shutil.which("ffmpeg")
if not ff:
    raise SystemExit("FFmpeg is required")


def run(args, name):
    subprocess.run([ff, "-hide_banner", "-loglevel", "error", "-y", *args,
                    "-threads", "2", str(out / name)], check=True)


def scene(seed, name):
    rng = random.Random(seed)
    w, h, fps = 320, 180, 12
    shapes = [(rng.randrange(w), rng.randrange(h), rng.randrange(15, 80),
               rng.randrange(10, 60), bytes([rng.randrange(35, 245) for _ in range(3)]))
              for _ in range(32)]
    cmd = [ff, "-hide_banner", "-loglevel", "error", "-y", "-f", "rawvideo",
           "-pix_fmt", "rgb24", "-s", f"{w}x{h}", "-r", str(fps), "-i", "pipe:0",
           "-vf", "scale=1920:1080:flags=bicubic", "-c:v", "libx264", "-preset", "veryfast",
           "-crf", "20", "-pix_fmt", "yuv420p", "-threads", "2", "-movflags", "+faststart",
           "-metadata", "comment=DDG deterministic validation source", str(out / name)]
    p = subprocess.Popen(cmd, stdin=subprocess.PIPE)
    for frame in range(12 * fps):
        canvas = bytearray(bytes([20, 32, 45]) * (w * h))
        for i, (x0, y0, sw, sh, color) in enumerate(shapes):
            x = int(x0 + 45 * math.sin(frame / fps * (0.2 + i / 45) + i)) % w
            y = int(y0 + 25 * math.cos(frame / fps * .6 + i)) % h
            for yy in range(y, min(h, y + sh)):
                start = (yy * w + x) * 3
                canvas[start:start + min(sw, w - x) * 3] = color * min(sw, w - x)
        p.stdin.write(canvas)
    p.stdin.close()
    if p.wait() != 0:
        raise SystemExit("Source encoding failed")


scene(17, "source.mp4")
scene(71, "similar_but_different.mp4")
run(["-f", "lavfi", "-i", "testsrc2=s=1920x1080:r=12:d=12", "-c:v", "libx264",
     "-preset", "veryfast", "-crf", "38", "-movflags", "+faststart"], "different.mp4")
transforms = [
    ("remux.mkv", ["-c", "copy"]),
    ("hevc.mp4", ["-c:v", "libx265", "-preset", "fast", "-x265-params", "pools=2:frame-threads=1:log-level=error", "-crf", "27", "-movflags", "+faststart"]),
    ("720p.mp4", ["-vf", "scale=1280:720", "-c:v", "libx264", "-crf", "25"]),
    ("bitrate.mp4", ["-c:v", "libx264", "-crf", "38"]),
    ("metadata.mp4", ["-map_metadata", "-1", "-c", "copy"]),
    ("trim.mp4", ["-ss", "2", "-c:v", "libx264", "-crf", "23"]),
    ("watermark.mp4", ["-vf", "drawbox=x=iw-180:y=20:w=160:h=55:color=white@0.65:t=fill", "-c:v", "libx264"]),
    ("crop.mp4", ["-vf", "crop=1824:1026,scale=1920:1080", "-c:v", "libx264"]),
    ("brightness.mp4", ["-vf", "eq=brightness=0.05:contrast=1.07", "-c:v", "libx264"]),
]
for name, args in transforms:
    run(["-i", str(out / "source.mp4"), *args], name)
run(["-ss", "5", "-i", str(out / "source.mp4"), "-frames:v", "1"], "source.png")
for name, args in [("recompressed.jpg", ["-q:v", "12"]),
                   ("resized.png", ["-vf", "scale=640:360"]),
                   ("converted.webp", ["-c:v", "libwebp", "-quality", "65"]),
                   ("cropped.png", ["-vf", "crop=1824:1026,scale=1920:1080"])]:
    run(["-i", str(out / "source.png"), *args, "-frames:v", "1"], name)
cases = []


def case(label, remote, expected, local="source.mp4", remote_name=None, local_name=None):
    cases.append(dict(label=label, remote=remote, local=local, expected=expected,
                      remoteName=remote_name or remote, localName=local_name or local))


case("A identical", "source.mp4", "exact")
case("B renamed", "source.mp4", "exact", remote_name="VID_20240321_183512.mp4")
case("C moved", "source.mp4", "exact", local_name="moved/renamed.mp4")
for label, f in [("D remux", "remux.mkv"), ("E H264-H265", "hevc.mp4"),
                 ("F 1080-720", "720p.mp4"), ("G bitrate", "bitrate.mp4"),
                 ("H metadata", "metadata.mp4"), ("I trim-2s", "trim.mp4"),
                 ("J watermark", "watermark.mp4")]:
    case(label, f, "related", remote_name="unrelated_name" + Path(f).suffix)
case("K similar but different", "similar_but_different.mp4", "different")
case("L JPEG recompression", "recompressed.jpg", "related", local="source.png")
case("M image resize", "resized.png", "related", local="source.png")
case("N different same duration", "different.mp4", "different")
case("O same name different content", "different.mp4", "different", remote_name="source.mp4")
case("P minor crop", "crop.mp4", "related")
case("Q brightness", "brightness.mp4", "related")
case("R WebP conversion", "converted.webp", "related", local="source.png")
case("S image crop", "cropped.png", "related", local="source.png")
(out / "cases.json").write_text(json.dumps(cases, indent=2), encoding="utf-8")
manifest = {p.name: hashlib.sha256(p.read_bytes()).hexdigest() for p in out.iterdir() if p.is_file()}
(out / "manifest.json").write_text(json.dumps(manifest, indent=2), encoding="utf-8")
print(f"Generated {len(cases)} cases at {out}", flush=True)
