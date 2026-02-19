package handler

import (
	"context"
	"crypto/rand"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"regexp"
	"strings"
	"time"

	"sms_service/socketserver"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

// Patterns mirror the original Node.js regexes exactly.
var (
	phonePattern   = regexp.MustCompile(`^[6][1-5][0-9]{6}$`)
	sendSMSPattern = regexp.MustCompile(`^(\+993)?6[1-5]\d{6}`)
)

const (
	otpTTLSeconds time.Duration = 1800
	otpKeyPrefix                = "otp:"
)

// Handler holds shared dependencies for all HTTP handlers.
type Handler struct {
	redis  *redis.Client
	socket *socketserver.Manager
}

// New creates a Handler with the given dependencies.
func New(rdb *redis.Client, sm *socketserver.Manager) *Handler {
	return &Handler{redis: rdb, socket: sm}
}

// OTP handles POST /otp.
// Generates a 5-digit code, stores it in Redis for 30 min, and emits
// the "otp" Socket.IO event to all connected clients.
func (h *Handler) OTP(c *gin.Context) {
	ip := c.ClientIP()
	log.Printf("[OTP] Request received | ip=%s", ip)

	var body struct {
		Phone string `json:"phone"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		log.Printf("[OTP] Failed to parse request body | ip=%s | error=%v", ip, err)
		c.JSON(http.StatusBadRequest, gin.H{"message": "Bad request"})
		return
	}
	if !phonePattern.MatchString(body.Phone) {
		log.Printf("[OTP] Invalid phone number | ip=%s | phone=%q", ip, body.Phone)
		c.JSON(http.StatusBadRequest, gin.H{"message": "Bad request"})
		return
	}

	ctx := context.Background()
	key := otpKeyPrefix + body.Phone

	// If an OTP already exists, tell the caller to wait.
	existing, err := h.redis.Get(ctx, key).Result()
	if err != nil && err != redis.Nil {
		log.Printf("[OTP] Redis GET error | ip=%s | phone=%s | error=%v", ip, body.Phone, err)
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}
	if err == nil && existing != "" {
		log.Printf("[OTP] OTP already active, rejecting | ip=%s | phone=%s", ip, body.Phone)
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "OTP already sent. Please wait.",
		})
		return
	}

	code, err := generateOTP()
	if err != nil {
		log.Printf("[OTP] Failed to generate OTP | ip=%s | phone=%s | error=%v", ip, body.Phone, err)
		c.JSON(http.StatusInternalServerError, gin.H{"message": "Failed to generate OTP"})
		return
	}

	log.Printf("[OTP] Emitting OTP event via socket | ip=%s | phone=+993%s", ip, body.Phone)
	h.socket.Emit("otp", socketserver.OTPEvent{
		Phone: fmt.Sprintf("+993%s", body.Phone),
		Pass:  fmt.Sprintf("Siziň aktiwasiýa koduňyz %s", code),
	})

	if err := h.redis.SetEx(ctx, key, code, otpTTLSeconds*time.Second).Err(); err != nil {
		log.Printf("[OTP] Redis SETEX error | ip=%s | phone=%s | error=%v", ip, body.Phone, err)
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}

	log.Printf("[OTP] OTP stored and sent successfully | ip=%s | phone=%s | ttl=%ds", ip, body.Phone, otpTTLSeconds)
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// Compare handles POST /compare.
// Verifies the submitted OTP against the value stored in Redis.
func (h *Handler) Compare(c *gin.Context) {
	ip := c.ClientIP()
	log.Printf("[COMPARE] Request received | ip=%s", ip)

	var body struct {
		Phone string `json:"phone"`
		Pass  string `json:"pass"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		log.Printf("[COMPARE] Failed to parse request body | ip=%s | error=%v", ip, err)
		c.JSON(http.StatusBadRequest, gin.H{"message": "Bad request"})
		return
	}

	ctx := context.Background()
	key := otpKeyPrefix + body.Phone

	cached, err := h.redis.Get(ctx, key).Result()
	if err == redis.Nil {
		log.Printf("[COMPARE] OTP not found or expired | ip=%s | phone=%s", ip, body.Phone)
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "OTP expired"})
		return
	}
	if err != nil {
		log.Printf("[COMPARE] Redis GET error | ip=%s | phone=%s | error=%v", ip, body.Phone, err)
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}

	if body.Pass != cached {
		log.Printf("[COMPARE] Invalid OTP attempt | ip=%s | phone=%s", ip, body.Phone)
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "Invalid OTP"})
		return
	}

	if err := h.redis.Del(ctx, key).Err(); err != nil {
		log.Printf("[COMPARE] Redis DEL error | ip=%s | phone=%s | error=%v", ip, body.Phone, err)
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}

	log.Printf("[COMPARE] OTP verified and cleared | ip=%s | phone=%s", ip, body.Phone)
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// GroupSMS handles POST /group_sms.
// Emits a custom message to all connected clients via Socket.IO.
func (h *Handler) GroupSMS(c *gin.Context) {
	ip := c.ClientIP()
	log.Printf("[GROUP_SMS] Request received | ip=%s", ip)

	var body struct {
		Phone   string `json:"phone"`
		Message string `json:"message"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		log.Printf("[GROUP_SMS] Failed to parse request body | ip=%s | error=%v", ip, err)
		c.JSON(http.StatusBadRequest, gin.H{"message": "Bad request: Invalid phone number"})
		return
	}
	if !phonePattern.MatchString(body.Phone) {
		log.Printf("[GROUP_SMS] Invalid phone number | ip=%s | phone=%q", ip, body.Phone)
		c.JSON(http.StatusBadRequest, gin.H{"message": "Bad request: Invalid phone number"})
		return
	}

	phone := fmt.Sprintf("+993%s", body.Phone)

	log.Printf("[GROUP_SMS] Emitting group SMS via socket | ip=%s | phone=%s | message_len=%d", ip, phone, len(body.Message))
	h.socket.Emit("otp", socketserver.OTPEvent{
		Phone: phone,
		Pass:  body.Message,
	})

	log.Printf("[GROUP_SMS] Group SMS sent successfully | ip=%s | phone=%s", ip, phone)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Group SMS sent successfully",
		"phone":   phone,
	})
}

// SendSMS handles POST /send-sms.
// Accepts phone numbers with or without the +993 prefix.
func (h *Handler) SendSMS(c *gin.Context) {
	ip := c.ClientIP()
	log.Printf("[SEND_SMS] Request received | ip=%s", ip)

	var body struct {
		Phone   string `json:"phone"`
		Message string `json:"message"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		log.Printf("[SEND_SMS] Failed to parse request body | ip=%s | error=%v", ip, err)
		c.JSON(http.StatusBadRequest, gin.H{"message": "Bad request"})
		return
	}
	if !sendSMSPattern.MatchString(body.Phone) {
		log.Printf("[SEND_SMS] Invalid phone number | ip=%s | phone=%q", ip, body.Phone)
		c.JSON(http.StatusBadRequest, gin.H{"message": "Bad request"})
		return
	}

	phone := strings.TrimPrefix(body.Phone, "+993")
	fullPhone := fmt.Sprintf("+993%s", phone)

	log.Printf("[SEND_SMS] Emitting SMS via socket | ip=%s | phone=%s | message_len=%d", ip, fullPhone, len(body.Message))
	h.socket.Emit("otp", socketserver.OTPEvent{
		Phone: fullPhone,
		Pass:  body.Message,
	})

	log.Printf("[SEND_SMS] SMS sent successfully | ip=%s | phone=%s", ip, fullPhone)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Message sent",
		"phone":   fullPhone,
		"pass":    body.Message,
	})
}

// generateOTP returns a zero-padded 5-digit OTP string in the range [10000, 99999].
// Uses crypto/rand for cryptographic safety.
func generateOTP() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(90000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%d", n.Int64()+10000), nil
}
