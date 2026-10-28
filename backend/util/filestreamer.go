package util

import (
	"context"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"time"
)

type FileStreamerServer struct {
	Path       string
	IsComplete func() bool
	listener   net.Listener
	server     *http.Server
	done       chan struct{}
}

// NewFileStreamerServer creates a new server but doesn't start it yet.
func NewFileStreamerServer(path string, isComplete func() bool) (*FileStreamerServer, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}

	fs := &FileStreamerServer{
		Path:       path,
		IsComplete: isComplete,
		listener:   listener,
		done:       make(chan struct{}),
	}

	fs.server = &http.Server{
		Handler: handler{fs},
	}

	return fs, nil
}

// Addr returns the server address (host:port).
func (fs *FileStreamerServer) Addr() string {
	_, port, _ := net.SplitHostPort(fs.listener.Addr().String())
	return "http://127.0.0.1:" + port + "/"
}

// Serve starts serving and waits for a single request to complete,
// or for ctx to be canceled if the request never arrives.
func (fs *FileStreamerServer) Serve(ctx context.Context) error {
	go func() {
		_ = fs.server.Serve(fs.listener)
	}()

	select {
	case <-fs.done: // wait for the handler to finish
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	return fs.server.Shutdown(shutdownCtx)
}

// Handler that streams the file using chunked transfer encoding.
func (fs *FileStreamerServer) streamHandler(w http.ResponseWriter, _ *http.Request) {
	defer close(fs.done) // signal Serve() to shut down after this request

	file, err := os.Open(fs.Path)
	if err != nil {
		log.Println("File streamer failed to open source file")
		http.Error(w, "could not open file", http.StatusInternalServerError)
		return
	}
	defer file.Close()

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Transfer-Encoding", "chunked")
	w.WriteHeader(http.StatusOK)

	flusher, canFlush := w.(http.Flusher)

	bytesRead := int64(0)
	buf := make([]byte, 4096)
	for {
		complete := fs.IsComplete()
		if !complete {
			if s, err := os.Stat(fs.Path); err == nil {
				// make sure we don't read near EOF until file is complete
				maxToRead := max(0, s.Size()-bytesRead-1024) /*safety buffer*/
				buf = buf[:min(int64(cap(buf)), maxToRead)]
			}
		} else {
			buf = buf[:cap(buf)]
		}
		n, err := file.Read(buf)
		if err != nil && err != io.EOF {
			log.Printf("read error: %v", err)
			break
		}
		bytesRead += int64(n)

		if n > 0 {
			_, err := w.Write(buf[:n])
			if err != nil {
				log.Printf("client write error: %v", err)
				break
			}
			if canFlush {
				flusher.Flush()
			}
		}

		if n == 0 && complete {
			break
		}

		// Wait for more content to be written to the source file
		if n == 0 {
			time.Sleep(50 * time.Millisecond)
		}
	}
}

type handler struct {
	fs *FileStreamerServer
}

var _ http.Handler = handler{}

func (h handler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	h.fs.streamHandler(w, req)
}
