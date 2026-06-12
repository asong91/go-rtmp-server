# go-rtmp-server

An RTMP ingestion server written in Go from scratch.
This is a hobby project where I wanted to learn Golang and how RTMP works

Handles the full ingest lifecycle: handshake → chunk parsing → AMF0 command negotiation → audio/video stream receive.

## Reference

- [RTMP Spec](https://github.com/melpon/rfc/blob/master/rtmp.md)
- [AMF Spec](https://ossrs.net/lts/en-us/assets/files/rtmp.part3.Commands-Messages-4d897bbc8ee9e1401adb8cbea85c5dac.pdf)

## Run

```bash
# Start server
go run .

# Stream into it
ffmpeg -re -i assets/sample.mp4 -c copy -f flv rtmp://localhost:8080/
```

## TODO:

- Gracefully cut off connection if client stream suddenly interrupts. Right now the server crashes
- Bitrate & Latency configurations
- Save to file and different file types (probably different transcoding pipeline project)
- Output to HLS
