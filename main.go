package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"
	"time"

	"sms_service/config"
	"sms_service/handler"
	"sms_service/middleware"
	"sms_service/redisclient"
	"sms_service/socketserver"

	"github.com/gin-gonic/gin"
)

func main() {
	// Include date+time+file:line in every log line so crashes are easy to locate.
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	// Catch any panic that bubbles up to the main goroutine itself.
	// go-socket.io v1.7.0 internal goroutine panics will NOT be caught here
	// (each goroutine needs its own recover), but this is a last-resort safety net.
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[PANIC] main() goroutine panic – stack:\n%v\n%s", r, debug.Stack())
			os.Exit(1)
		}
	}()

	log.Printf("[STARTUP] Loading configuration...")
	cfg := config.Load()
	log.Printf("[STARTUP] Config loaded | port=%s | redis=%s:%s",
		cfg.Port, cfg.RedisHost, cfg.RedisPort)

	rdb := redisclient.NewClient(cfg)

	log.Printf("[STARTUP] Initializing Socket.IO manager...")
	sm := socketserver.NewManager()
	h := handler.New(rdb, sm)

	// Start the Socket.IO serve loop.
	// recover() here catches panics inside the Serve() loop itself.
	// Panics in go-socket.io's per-connection goroutines are separate and will
	// still crash the process — that is a known bug in go-socket.io v1.7.0.
	// Docker's --restart unless-stopped handles the crash+restart automatically.
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[SOCKET][PANIC] Serve() goroutine panicked | panic=%v\nstack:\n%s",
					r, debug.Stack())
			}
		}()
		log.Printf("[STARTUP] Socket.IO serve loop starting...")
		if err := sm.Server.Serve(); err != nil {
			log.Printf("[SOCKET] Serve() returned error | error=%v", err)
		}
	}()
	defer sm.Server.Close()

	gin.SetMode(gin.ReleaseMode)

	router := gin.New()
	router.Use(gin.Logger())
	// gin.Recovery already catches panics in HTTP handler goroutines and logs them.
	router.Use(gin.Recovery())

	router.Use(middleware.SecurityHeaders())
	router.Use(middleware.CORS())

	// Health check — first thing to call when debugging ECONNRESET.
	// If this returns 200 the server is alive. If it times out, the server crashed.
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// Socket.IO — both polling and WebSocket upgrade.
	router.GET("/socket.io/*any", gin.WrapH(sm.Server))
	router.POST("/socket.io/*any", gin.WrapH(sm.Server))

	// REST API routes.
	router.POST("/otp", h.OTP)
	router.POST("/compare", h.Compare)
	router.POST("/group_sms", h.GroupSMS)
	router.POST("/send-sms", h.SendSMS)

	addr := fmt.Sprintf("0.0.0.0:%s", cfg.Port)

	srv := &http.Server{
		Addr:    addr,
		Handler: router,
		// Only timeout the header read to guard against Slowloris attacks.
		// ReadTimeout / WriteTimeout would kill long-lived WebSocket connections.
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	go func() {
		log.Printf("[STARTUP] HTTP server listening | addr=%s", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[STARTUP] Server failed | addr=%s | error=%v", addr, err)
		}
	}()

	// Block until SIGINT or SIGTERM (Ctrl-C / docker stop).
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit
	log.Printf("[SHUTDOWN] Signal received: %s – shutting down gracefully...", sig)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("[SHUTDOWN] Forced shutdown | error=%v", err)
	} else {
		log.Printf("[SHUTDOWN] Server stopped cleanly")
	}
}
