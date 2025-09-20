package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

func WithLogger(r *http.Request, l *log.Logger) *http.Request {
	ctx := context.WithValue(r.Context(), loggerKey, l)
	return r.WithContext(ctx)
}

func LoggerFrom(r *http.Request) *log.Logger {
	if l, ok := r.Context().Value(loggerKey).(*log.Logger); ok && l != nil {
		return l
	}
	panic("logger not found in request context")
}

type logger struct {
	*log.Logger
}

func newLogger() (*logger, error) {
	date := time.Now().Format("2006-01-02")
	filename := fmt.Sprintf("logs/app_log_%s.log", date)
	dir := filepath.Dir(filename)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}
	return &logger{log.New(f, "", log.LstdFlags)}, nil
}

func loggingMiddleware(next http.Handler, log *logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		r = WithLogger(r, log.Logger)
		next.ServeHTTP(w, r)
		duration := time.Since(start)
		log.Printf("%s %s %s %v", r.RemoteAddr, r.Method, r.URL.Path, duration)
	})
}