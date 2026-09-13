package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"boot.dev/linko/internal/store"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)

	httpPort := flag.Int("port", 8899, "port to listen on")
	dataDir := flag.String("data", "./data", "directory to store data")
	flag.Parse()

	status := run(ctx, cancel, *httpPort, *dataDir)
	cancel()
	os.Exit(status)
}

func run(ctx context.Context, cancel context.CancelFunc, httpPort int, dataDir string) int {
	logFileStr := os.Getenv("LINKO_LOG_FILE")
	logger, closeFunc, err := initializeLogger(logFileStr)
	defer func() {
		err := closeFunc()
		if err != nil {
			fmt.Printf("failed to close logger: %v\n", err)
		}
	}()
	if err != nil {
		fmt.Printf("failed to initialize logger: %v", err)
		return 1
	}
	st, err := store.New(dataDir, logger)
	if err != nil {
		logger.Printf("failed to create store: %v", err)
		return 1
	}
	s := newServer(*st, httpPort, cancel, logger)
	var serverErr error
	go func() {
		serverErr = s.start()
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	logger.Println("Linko is shutting down")
	if err := s.shutdown(shutdownCtx); err != nil {
		logger.Printf("failed to shutdown server: %v", err)
		return 1
	}
	if serverErr != nil {
		logger.Printf("server error: %v", serverErr)
		return 1
	}
	return 0
}

type closeFunc func() error

func initializeLogger(logFileStr string) (*log.Logger, closeFunc, error) {
	if logFileStr != "" {
		logFile, err := os.OpenFile(logFileStr, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o604)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to open log file: %v", err)
		}
		bufLogFile := bufio.NewWriterSize(logFile, 8192)
		multiWriter := io.MultiWriter(os.Stderr, bufLogFile)
		close := func() error {
			if err := bufLogFile.Flush(); err != nil {
				return fmt.Errorf("failed to flush log file: %v", err)
			}
			if err := logFile.Close(); err != nil {
				return fmt.Errorf("failed to close log file: %v", err)
			}
			return nil
		}
		return log.New(multiWriter, "", log.LstdFlags), close, nil
	}
	return log.New(os.Stderr, "", log.LstdFlags), func() error { return nil }, nil
}
