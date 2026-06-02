package handler

import (
	"embed"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/kudryavtsevmakar/valovoting/internal/poll"
)

type Handler struct {
	pollSvc     *poll.Service
	staticFiles embed.FS
}

func New(pollSvc *poll.Service, staticFiles embed.FS) *Handler {
	return &Handler{
		pollSvc:     pollSvc,
		staticFiles: staticFiles,
	}
}

func (h *Handler) GetPollState(c *gin.Context) {
	c.JSON(http.StatusOK, h.pollSvc.GetState())
}

func (h *Handler) ServeOverlay(c *gin.Context) {
	data, err := h.staticFiles.ReadFile("static/overlay.html")
	if err != nil {
		c.Status(http.StatusInternalServerError)
		return
	}
	c.Data(http.StatusOK, "text/html; charset=utf-8", data)
}
