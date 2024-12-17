#!/usr/bin/python3
from yt_dlp import YoutubeDL
import requests
import re
import threading
import spotify.sync as spotify
import toml
import music_tag
import sys

nsongs = 0
root = "/srv/music"
authfile = toml.load("auth.toml")
client_id = authfile["spotify"]["client_id"]
client_secret = authfile["spotify"]["client_secret"]


def get_bigrams(string):
    """
    Take a string and return a list of bigrams.
    """
    s = string.lower()
    return [s[i : i + 2] for i in list(range(len(s) - 1))]


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


def get_metadata(filename):
    client = spotify.Client(client_id, client_secret)
    query = filename.split("/")[-1]
    print(f"Searching for {query} on Spotify...")
    search = client.search(query, types=["track"])
    tracks = search.tracks
    max = 0
    song = None
    for track in tracks:
        score = rank(
            filename,
            f"{track.name} - {track.artists[0].name}",
        )
        print(f"Track: {track.name} - {track.artists[0]} Score: {score}")
        if score > max:
            song = track
            max = score
        else:
            continue
    cover = song.images[0].url
    print(cover)
    title = song.name
    album = song.album.name
    track_number = song.track_number
    artists = ""
    for artist in song.artists:
        artists += artist.name + ", "
    artists = artists[:-2]
    r = requests.get(cover)
    cover_bytes = r.content
    f = music_tag.load_file(filename)
    f["title"] = title
    f["album"] = album
    f["tracknumber"] = track_number
    f["artist"] = artists
    f["artwork"] = cover_bytes
    f.save()
    a = client.close()


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
    get_metadata(f"{root}/{song}.mp3")
