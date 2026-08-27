package main

import (
	"benzhi-project-1a5cefa6-5929-4903-a02e-a30af7b56e19/internal/application"
	"benzhi-project-1a5cefa6-5929-4903-a02e-a30af7b56e19/internal/store"
	webui "benzhi-project-1a5cefa6-5929-4903-a02e-a30af7b56e19/internal/web"
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
func run() error {
	cfg, err := parseConfig()
	if err != nil {
		return err
	}
	dataDir := cfg.dataDir
	if cfg.selfcheck {
		dataDir, err = os.MkdirTemp("", "smoke-qualification-*")
		if err != nil {
			return err
		}
		defer os.RemoveAll(dataDir)
	}
	repo, err := store.New(dataDir)
	if err != nil {
		return err
	}
	if err = repo.VerifyAll(); err != nil {
		return err
	}
	app := application.New(repo)
	handler := webui.New(app)
	listener, err := net.Listen("tcp", cfg.addr)
	if err != nil {
		return fmt.Errorf("监听 %s: %w", cfg.addr, err)
	}
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second}
	serveErr := make(chan error, 1)
	go func() {
		e := server.Serve(listener)
		if e != nil && !errors.Is(e, http.ErrServerClosed) {
			serveErr <- e
		}
		close(serveErr)
	}()
	if cfg.selfcheck {
		err = runSelfcheck("http://" + listener.Addr().String())
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		shutdownErr := server.Shutdown(ctx)
		if err != nil {
			return err
		}
		if shutdownErr != nil {
			return shutdownErr
		}
		fmt.Println("自检通过：失败、整改、定向复演、批准和证书校验链路完整")
		return nil
	}
	log.Printf("%s", webui.HealthText(listener.Addr().String()))
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	select {
	case <-signals:
	case err = <-serveErr:
		if err != nil {
			return err
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return server.Shutdown(ctx)
}
