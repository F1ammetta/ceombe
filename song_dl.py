#!/usr/bin/python3
from yt_dlp import YoutubeDL
import toml
import sys
import os

root = toml.load("config.toml")["Server"]["music_dir"]

def my_hook(d):
    if d["status"] == "finished":
        # The postprocessor converts the file to mp3, so we manually update the filename.
        base, ext = os.path.splitext(d["filename"])
        mp3_song = base + '.mp3'
        print(mp3_song, end="")


url = sys.argv[1]
ydl_opts = {
    "format": "bestaudio/best",
    "quiet": True,
    "noprogress": True,
    "progress_hooks": [my_hook],
    "outtmpl": f"{root}/%(title)s - %(uploader)s.%(ext)s",
    "postprocessors": [
        {
            "key": "FFmpegExtractAudio",
            "preferredcodec": "mp3",
            "preferredquality": "320",
        },
        {"key": "FFmpegMetadata"},
    ],
    "writethumbnail": False,
}

with YoutubeDL(ydl_opts) as ydl:
    ydl.download([url])