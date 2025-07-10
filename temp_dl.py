#!/usr/bin/python3
from yt_dlp import YoutubeDL
import sys
import os

# A hook to capture the final filename after post-processing
def my_hook(d):
    if d['status'] == 'finished':
        # After ffmpeg converts the file, the extension changes.
        base, _ = os.path.splitext(d['filename'])
        mp3_snippet = base + '.mp3'
        print(mp3_snippet, end='')

url = sys.argv[1]

ydl_opts = {
    'format': 'bestaudio/best',
    'quiet': True,
    'noprogress': True,
    'progress_hooks': [my_hook],
    # Use a temp directory and a unique ID for the output file
    'outtmpl': '/tmp/%(id)s.%(ext)s',
    'postprocessors': [
        {
            # Use ffmpeg to extract the audio
            'key': 'FFmpegExtractAudio',
            'preferredcodec': 'mp3',
            'preferredquality': '128', # Lower quality for a snippet is fine
        },
        {
            # Use ffmpeg to trim the audio to a 20-second snippet from the beginning
            'key': 'FFmpegSubtitlesConvertor',
            'from': 'm4a',
            'to': 'mp3',
        },
        {
            'key': 'FFmpegMetadata',
        },
    ],
    # Download only the first 30 seconds to be safe
    'download_ranges': lambda info_dict, ydl: ydl.download_range_func(info_dict, [{'start_time': 0, 'end_time': 30}]),
    'writethumbnail': False,
}

with YoutubeDL(ydl_opts) as ydl:
    ydl.download([url])
