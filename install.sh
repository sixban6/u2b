#!/usr/bin/env bash
brew install yt-dlp || brew upgrade yt-dlp
GOPROXY=direct go install git.sr.ht/~sixban/u2b_download@main