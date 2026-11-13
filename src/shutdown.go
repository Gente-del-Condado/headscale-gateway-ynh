package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

var processContext = context.Background()
var shutdownTasks sync.WaitGroup

func shutdownHTTPServer(server *http.Server) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = server.Shutdown(ctx)
}

func waitForShutdown() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(c)
	select {
	case <-c:
	case <-processContext.Done():
	}
}
