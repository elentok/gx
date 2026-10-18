# /// script
# dependencies = ["numpy"]
# ///
"""Regenerate sounds/done.m4a: a soft rising sine chime (D5 -> G5).

Run from the repo root (needs uv and ffmpeg):

    uv run sound/gen_done.py
"""

import subprocess
import tempfile
import wave
from pathlib import Path

import numpy as np

SR = 44100
PEAK_DB = -10.0  # quiet, close to herdr's done sound
OUT = Path(__file__).parent / "sounds" / "done.m4a"


def note(freq, dur, decay):
    t = np.arange(int(SR * dur)) / SR
    attack = np.minimum(t / 0.006, 1.0)
    return np.sin(2 * np.pi * freq * t) * attack * np.exp(-t / decay)


def mix(parts, total):
    out = np.zeros(int(SR * total))
    for start, sig in parts:
        i = int(SR * start)
        n = min(len(sig), len(out) - i)
        out[i : i + n] += sig[:n]
    fade = int(SR * 0.05)
    out[-fade:] *= np.linspace(1, 0, fade)
    return out


def main():
    x = mix([(0.0, note(587, 0.8, 0.10)), (0.11, note(784, 0.8, 0.16))], 0.9)
    x = x / np.abs(x).max() * 10 ** (PEAK_DB / 20)
    pcm = (x * 32767).astype(np.int16)
    with tempfile.TemporaryDirectory() as tmp:
        wav = Path(tmp) / "done.wav"
        with wave.open(str(wav), "wb") as w:
            w.setnchannels(1)
            w.setsampwidth(2)
            w.setframerate(SR)
            w.writeframes(pcm.tobytes())
        # afplay cannot play Ogg Vorbis, so the built-ins are AAC.
        subprocess.run(
            ["ffmpeg", "-loglevel", "error", "-y", "-i", str(wav), "-c:a", "aac", "-b:a", "96k", str(OUT)],
            check=True,
        )


if __name__ == "__main__":
    main()
