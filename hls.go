package main

import (
	"fmt"
	"net"
	"net/http"
)

func test(w http.ResponseWriter, req *http.Request) {
	fmt.Fprintf(w, "hello from hls server")
}

func serve(w http.ResponseWriter, req *http.Request) {
	steamName := req.PathValue("streamName")
	fileName := req.PathValue("fileName")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	http.ServeFile(w, req, "streams/"+steamName+"/"+fileName)

}

func headers(w http.ResponseWriter, req *http.Request) {

	for name, headers := range req.Header {
		for _, h := range headers {
			fmt.Fprintf(w, "%v: %v\n", name, h)
		}
	}
}

func serveHLS(listener net.Listener) error {
	fmt.Println("Starting hls server")

	mux := http.NewServeMux()
	mux.HandleFunc("GET /stream/{streamName}/{fileName}", serve)
	mux.HandleFunc("GET /test", test)
	return fmt.Errorf("hls server: %w", http.Serve(listener, mux)) // Serve only returns on failure

}
