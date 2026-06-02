package main

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"github.com/kudryavtsevmakar/valovoting/api/twitch"
	"github.com/kudryavtsevmakar/valovoting/internal/config"
	"github.com/kudryavtsevmakar/valovoting/internal/handler"
	"github.com/kudryavtsevmakar/valovoting/internal/hub"
	"github.com/kudryavtsevmakar/valovoting/internal/poll"
	"github.com/kudryavtsevmakar/valovoting/internal/setup"
)

//go:embed static/overlay.html
var staticFiles embed.FS

func main() {
	setup.PrintBanner()

	// Run interactive wizard if .env is missing or incomplete
	if setup.IsNeeded() {
		if err := setup.Run(); err != nil {
			log.Fatalf("setup: %v", err)
		}
	}

	_ = godotenv.Load()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	gin.SetMode(gin.ReleaseMode)

	pollService := poll.NewService()
	h := hub.New()

	onUpdate := func(st poll.State) {
		type wsMsg struct {
			Type string     `json:"type"`
			Data poll.State `json:"data"`
		}
		b, _ := json.Marshal(wsMsg{Type: "poll_update", Data: st})
		h.Broadcast(b)
	}

	evClient := twitch.NewEventSubClient(cfg, pollService, onUpdate)
	pollAPI := twitch.NewPollAPI(cfg)
	chatBot := twitch.NewChatBot(cfg, pollAPI)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go h.Run()
	go evClient.Run(ctx)
	go chatBot.Run(ctx)

	apiHandler := handler.New(pollService, staticFiles)
	r := handler.NewRouter(apiHandler, h)

	fmt.Printf("  Оверлей:  http://localhost:%s/overlay\n\n", cfg.Port)

	srv := &http.Server{Addr: ":" + cfg.Port, Handler: r}

	go func() {
		log.Printf("server: listening on :%s", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("server: shutting down")
	cancel()
	shutCtx, shutCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutCancel()
	_ = srv.Shutdown(shutCtx)
}
