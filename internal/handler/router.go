package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/kudryavtsevmakar/valovoting/internal/hub"
)

func NewRouter(h *Handler, wsHub *hub.Hub) *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(corsMiddleware())

	r.GET("/api/poll/state", h.GetPollState)
	r.GET("/ws", wsHub.ServeWS)
	r.GET("/overlay", h.ServeOverlay)

	return r
}

func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}
