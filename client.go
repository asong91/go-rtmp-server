package main

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"net"
	"os"
	"os/exec"
	"path/filepath"
)

type Client struct {
	conn                  net.Conn
	chunkSize             int
	previousChunkHeaders  map[uint32]ChunkMessageHeader
	chunkStreamTimestamps map[uint32]uint32 // absolute timestamp per chunk stream id

	// RTMP Stream Data
	AudioSeqHeader      *Chunk
	VideoSeqHeader      *Chunk
	AudTimestamp        uint32
	VidTimestamp        uint32
	AudPacket           int
	VidPacket           int
	FlvOnMetaDataPacket FLVPacket
	durationIndex       int

	Output StreamOutput

	StreamConfig StreamConfig
}

type StreamConfig struct {
	ClientUrl  string
	OutputType string
	OutputDest string
	StreamId   string
}

type StreamOutput interface {
	createOutput(metadata FLVPacket, audHeader Chunk, vidHeader Chunk) error
	writeMedia(chunk Chunk, timestamp uint32) error
	closeStream() error
}

type FileOutput struct {
	fileName      string
	durationIndex int
	file          *os.File
	muxer         flvMuxer
}

func (f *FileOutput) createOutput(metadata FLVPacket, audHeader Chunk, vidHeader Chunk) error {
	if _, err := os.Stat(f.fileName); err == nil {
		return fmt.Errorf("file %s already exists, not recording", f.fileName)
	}
	file, err := os.Create(f.fileName)
	if err != nil {
		return err
	}
	f.file = file
	_, err = f.file.Write(f.muxer.createStartPackets(metadata, audHeader, vidHeader))
	return err
}

func (f *FileOutput) writeMedia(chunk Chunk, timestamp uint32) error {
	_, err := f.file.Write(f.muxer.createFLVPacket(chunk, timestamp))
	return err
}

// Patches the real duration into onMetaData, then closes the file
func (f *FileOutput) closeStream() error {
	// 0 means the metadata had no duration field, so there's nothing to patch
	if f.durationIndex > 0 {
		lastTimestamp := max(f.muxer.LastAudTagTimestamp, f.muxer.LastVidTagTimestamp)
		durationSeconds := float64(lastTimestamp) / 1000.0
		var buf [8]byte
		binary.BigEndian.PutUint64(buf[:], math.Float64bits(durationSeconds))
		if _, err := f.file.WriteAt(buf[:], int64(f.durationIndex)); err != nil {
			fmt.Println("Error writing duration:", err)
		}
	}
	return f.file.Close()
}

type HlsOutput struct {
	outDir string
	Cmd    *exec.Cmd
	Stdin  io.WriteCloser
	muxer  flvMuxer
}

func newFFmpegHLSCmd(outDir string) *exec.Cmd {
	return exec.Command("ffmpeg",
		"-f", "flv",
		"-i", "pipe:0",
		"-c", "copy",
		"-f", "hls",
		"-hls_time", "2",
		"-hls_list_size", "5",
		"-hls_flags", "delete_segments",
		"-hls_segment_filename", filepath.Join(outDir, "seg%d.ts"),
		filepath.Join(outDir, "stream.m3u8"),
	)
}

func (h *HlsOutput) createOutput(metadata FLVPacket, audHeader Chunk, vidHeader Chunk) error {
	if err := os.RemoveAll(h.outDir); err != nil {
		fmt.Println("Error removing dir:", err)
	}
	if err := os.MkdirAll(h.outDir, 0755); err != nil {
		return err
	}
	cmd := newFFmpegHLSCmd(h.outDir)
	cmd.Stderr = os.Stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}

	// Only store once ffmpeg is actually running
	h.Cmd = cmd
	h.Stdin = stdin

	_, err = h.Stdin.Write(h.muxer.createStartPackets(metadata, audHeader, vidHeader))
	return err
}

func (h *HlsOutput) writeMedia(chunk Chunk, timestamp uint32) error {
	_, err := h.Stdin.Write(h.muxer.createFLVPacket(chunk, timestamp))
	return err
}

// Closing stdin sends ffmpeg EOF so it flushes the last segment and writes #EXT-X-ENDLIST
func (h *HlsOutput) closeStream() error {
	h.Stdin.Close()
	return h.Cmd.Wait()
}
