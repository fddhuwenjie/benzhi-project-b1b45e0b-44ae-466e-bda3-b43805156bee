package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"icecoreverdict/internal/api"
	"icecoreverdict/internal/application"
	"icecoreverdict/internal/archive"
	"icecoreverdict/internal/storage"
)

func main() {
	addrFlag := flag.String("addr", defaultAddr, "回环监听地址")
	dataDir := flag.String("data-dir", "./data", "数据目录")
	selfCheck := flag.Bool("self-check", false, "运行真实 HTTP 全流程自检后退出")
	flag.Parse()
	addr, err := configuredAddr(*addrFlag)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	root := *dataDir
	if *selfCheck {
		root, err = os.MkdirTemp("", "icecore-verdict-self-check-*")
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		defer os.RemoveAll(root)
	}
	if err := run(addr, filepath.Clean(root), *selfCheck); err != nil {
		fmt.Fprintln(os.Stderr, "IceCoreVerdict 启动失败:", err)
		os.Exit(1)
	}
}

func run(addr, root string, selfCheck bool) error {
	store, err := storage.Open(root)
	if err != nil {
		return err
	}
	archives := archive.New(store)
	app := application.New(application.Services{Store: store, Archive: archives})
	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	handler := api.New(app, logger).Handler()
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 1 << 20}
	serveErr := make(chan error, 1)
	go func() {
		err := server.Serve(listener)
		if err != nil && err != http.ErrServerClosed {
			serveErr <- err
		} else {
			serveErr <- nil
		}
	}()
	if selfCheck {
		checkErr := runSelfCheck("http://" + listener.Addr().String())
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		shutdownErr := server.Shutdown(ctx)
		if checkErr != nil {
			return checkErr
		}
		if shutdownErr != nil {
			return shutdownErr
		}
		if err := <-serveErr; err != nil {
			return err
		}
		fmt.Println("IceCoreVerdict self-check passed")
		return nil
	}
	logger.Info("service_started", "addr", listener.Addr().String(), "data_dir", root)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	select {
	case <-ctx.Done():
	case err := <-serveErr:
		return err
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return server.Shutdown(shutdownCtx)
}
