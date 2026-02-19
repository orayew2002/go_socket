package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"sms_service/config"
	"sms_service/handler"
	"sms_service/middleware"
	"sms_service/redisclient"
	"sms_service/socketserver"
)

func main() {
	log.Printf("[STARTUP] Loading configuration...")
	cfg := config.Load()
	log.Printf("[STARTUP] Config loaded | port=%s | redis=%s:%s | allowed_origins=%v",
		cfg.Port, cfg.RedisHost, cfg.RedisPort, cfg.AllowedOrigins)

	rdb := redisclient.NewClient(cfg)

	log.Printf("[STARTUP] Initializing Socket.IO manager...")
	sm := socketserver.NewManager(cfg.AllowedOrigins)
	h := handler.New(rdb, sm)

	// Start the Socket.IO server in a background goroutine.
	go func() {
		log.Printf("[STARTUP] Socket.IO server starting...")
		if err := sm.Server.Serve(); err != nil {
			log.Printf("[SOCKET] Server stopped with error: %v", err)
		}
	}()
	defer sm.Server.Close()

	gin.SetMode(gin.ReleaseMode)

	router := gin.New()
	router.Use(gin.Logger())
	router.Use(gin.Recovery())

	// Global middleware – order mirrors the Node.js app.
	router.Use(middleware.SecurityHeaders())
	router.Use(middleware.CORS(cfg.AllowedOrigins))

	// Socket.IO – handle both polling (GET/POST) and WebSocket upgrades.
	router.GET("/socket.io/*any", gin.WrapH(sm.Server))
	router.POST("/socket.io/*any", gin.WrapH(sm.Server))

	// REST API routes.
	router.POST("/otp", h.OTP)
	router.POST("/compare", h.Compare)
	router.POST("/group_sms", h.GroupSMS)
	router.POST("/send-sms", h.SendSMS)

	addr := fmt.Sprintf("0.0.0.0:%s", cfg.Port)
	log.Printf("[STARTUP] HTTP server listening | addr=%s", addr)

	srv := &http.Server{
		Addr:    addr,
		Handler: router,
	}

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("[STARTUP] Server failed to start | addr=%s | error=%v", addr, err)
	}
}
