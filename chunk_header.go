package main

type chunkHeader struct {
	fmt           byte // only 2 bits
	chunkStreamID int  // at least 6 bits can be up to 6bits + 2 bytes
}
