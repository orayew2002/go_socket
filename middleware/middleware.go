package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// CORS allows requests from any origin.
func CORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")
		if origin != "" {
			// Echo the request origin so credentials work alongside the wildcard.
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Access-Control-Allow-Credentials", "true")
		} else {
			c.Header("Access-Control-Allow-Origin", "*")
		}
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")
		c.Header("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		c.Header("Vary", "Origin")

		// Handle preflight.
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}

// SecurityHeaders sets the same security headers that helmet.js applied in
// the Node.js version.
func SecurityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		// frameguard: deny
		c.Header("X-Frame-Options", "DENY")
		// noSniff
		c.Header("X-Content-Type-Options", "nosniff")
		// xssFilter
		c.Header("X-XSS-Protection", "1; mode=block")
		// hsts: maxAge=123456, includeSubDomains=false
		c.Header("Strict-Transport-Security", "max-age=123456")
		// referrerPolicy
		c.Header("Referrer-Policy", "origin, unsafe-url")
		// contentSecurityPolicy
		c.Header("Content-Security-Policy",
			"default-src 'self'; script-src 'self' securecoding.com")
		// dnsPrefetchControl: allow
		c.Header("X-DNS-Prefetch-Control", "on")
		// ieNoOpen
		c.Header("X-Download-Options", "noopen")
		// crossOriginOpenerPolicy
		c.Header("Cross-Origin-Opener-Policy", "same-origin")

		c.Next()
	}
}
