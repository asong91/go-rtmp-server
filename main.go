package main

import (
	"flag"
	"fmt"
	"log"
	"log/slog"
	"net"
)

const S1_C1_SIZE = 1536

var chunkSize = 4096

func main() {

	clientFlagPtr := flag.String("client", "localhost:8080", "client url.  defaults to localhost:8080")
	outputTypeFlagPtr := flag.String("oType", "file", "output type. hls or file")
	outputLocPtr := flag.String("output", "", "output file name or hls url. file defaults to output_vid<timestamp>.flv")

	flag.Parse()

	if *outputTypeFlagPtr != "file" && *outputTypeFlagPtr != "hls" {
		log.Fatal("oType flag must be file or hls")
	}

	slog.SetLogLoggerLevel(slog.LevelDebug)
	streamConfig := StreamConfig{ClientUrl: *clientFlagPtr, OutputType: *outputTypeFlagPtr, OutputDest: *outputLocPtr}
	if err := startup(streamConfig); err != nil {
		log.Fatal(err)
	}
}

// Starts up incoming RTMP TCP connection
// Starts ffmpeg exec
// Starts up outgoing HLS connection
// - Includes ffmpeg exec
func startup(sc StreamConfig) error {
	rtmpLn, err := net.Listen("tcp", sc.ClientUrl)
	if err != nil {
		return fmt.Errorf("rtmp listen on %q: %w", sc.ClientUrl, err)
	}
	defer rtmpLn.Close()

	errc := make(chan error, 2) // buffered: senders never block, even after run returns

	if sc.OutputType == "hls" {
		hlsLn, err := net.Listen("tcp", "localhost:9091")
		if err != nil {
			return fmt.Errorf("hls listen on %q: %w", sc.OutputDest, err)
		}
		defer hlsLn.Close()
		go func() { errc <- serveHLS(hlsLn) }()
	}
	go func() { errc <- listenRTMP(rtmpLn, sc) }()

	return <-errc // whichever server fails first ends the program

}

func listenRTMP(listener net.Listener, sc StreamConfig) error {
	// RTMP Connection
	fmt.Println("RTMP listening on", listener.Addr())
	for {
		c, err := listener.Accept()
		if err != nil {
			log.Fatal("Error accepting connection:", err)
		}
		client := Client{
			conn:                  c,
			chunkSize:             128,
			previousChunkHeaders:  make(map[uint32]ChunkMessageHeader),
			chunkStreamTimestamps: make(map[uint32]uint32),
			StreamConfig:          sc,
		}
		go client.rtmp()
	}
}

func (cl *Client) setTimestamp(c Chunk) {
	// Timestamp deltas are relative to the previous message on the same chunk
	// stream, whatever its type, so the running clock is kept per chunk stream.
	csId := c.ChunkHeader.ChunkBasicHeader.ChunkStreamId
	if c.ChunkHeader.ChunkBasicHeader.Fmt == 0 {
		cl.chunkStreamTimestamps[csId] = c.ChunkHeader.ChunkMessageHeader.Timestamp
	} else {
		cl.chunkStreamTimestamps[csId] += c.ChunkHeader.ChunkMessageHeader.Timestamp
	}

	if c.isAudioData() {
		cl.AudTimestamp = cl.chunkStreamTimestamps[csId]
	} else if c.isVideoData() {
		cl.VidTimestamp = cl.chunkStreamTimestamps[csId]
	}
}
