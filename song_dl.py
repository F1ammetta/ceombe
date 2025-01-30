#!/usr/bin/python3
from yt_dlp import YoutubeDL
import requests
import re
import threading
import subprocess
import toml
import music_tag
import sys

nsongs = 0
root = toml.load("config.toml")["Server"]["music_dir"]
authfile = toml.load("auth.toml")
client_id = authfile["spotify"]["client_id"]
client_secret = authfile["spotify"]["client_secret"]


def get_bigrams(string):
    """
    Take a string and return a list of bigrams.
    """
    s = string.lower()
    return [s[i: i + 2] for i in list(range(len(s) - 1))]


def rank(str1, str2):
    """
    Perform bigram comparison between two strings
    and return a percentage match in decimal form.
    """
    pairs1 = get_bigrams(str1)
    pairs2 = get_bigrams(str2)
    union = len(pairs1) + len(pairs2)
    hit_count = 0
    for x in pairs1:
        for y in pairs2:
            if x == y:
                hit_count += 1
                break
    return (2.0 * hit_count) / union


done = []


def my_hook(d):
    global done
    global done_song_file
    file_name = d["filename"]
    file_name = file_name.split("/")[-1]
    file_name = ".".join(file_name.split(".")[:-1])
    if d["status"] == "finished":
        done.append(file_name)


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
        # {"key": "EmbedThumbnail"},
    ],
    "writethumbnail": False,
}
print("\nDownloading ")
print("Please wait...\n")
with YoutubeDL(ydl_opts) as ydl:
    ydl.download([url])
print("Done downloading!\n")
print(done)
for song in done:
    # call ./metadata song to get the metadata of the song
    # if ".webm" in song:
    #     # replace webm with mp3
    #     song = song.replace(".webm", ".mp3")

    print(song)
    proc = subprocess.Popen(
        ["/home/silver/ceombe/chroma/metadata", f'{root}/{song}.mp3'],
        stdout=subprocess.PIPE, text=True, encoding="utf-8")
    while True:
        line = proc.stdout.readline()
        if not line:
            break
        # the real code does filtering here
        print("test:", line)
