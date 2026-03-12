package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"

	"github.com/peccancy/aliasb/internal/config"
	"github.com/peccancy/aliasb/internal/connection"
	"github.com/peccancy/aliasb/internal/handler"
	"github.com/peccancy/aliasb/internal/hub"
	"github.com/peccancy/aliasb/internal/service"
	"github.com/peccancy/aliasb/internal/store"
	"github.com/peccancy/aliasb/internal/words"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		panic("failed to load config: " + err.Error())
	}

	log := zerolog.New(os.Stdout).With().Timestamp().Logger()

	rdb := redis.NewClient(&redis.Options{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPassword,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Fatal().Err(err).Msg("failed to connect to redis")
	}

	wordsSvc, err := words.NewService()
	if err != nil {
		log.Fatal().Err(err).Msg("failed to load word dictionaries")
	}

	partnerClient := connection.NewPartnerClient(cfg.PartnerServiceURL)

	roomStore := store.NewRoomStore(rdb, cfg.RoomTTLHours)
	roomSvc := service.NewRoomService(roomStore, wordsSvc, partnerClient, cfg.PartnerID, cfg.PartnerAuthToken, log)
	wsHub := hub.NewHub(log)

	roomHandler := handler.NewRoomHandler(roomSvc, wsHub, log)
	wsHandler := handler.NewWSHandler(wsHub, roomSvc, log)

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Player-ID", "X-Partner-ID"},
		AllowCredentials: true,
	}))
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) })
	r.Get("/api/v1/stats", roomHandler.GetStats)
	r.Post("/api/v1/rooms", roomHandler.CreateRoom)
	r.Get("/api/v1/rooms/{id}", roomHandler.GetRoom)
	r.Post("/api/v1/rooms/{id}/start", roomHandler.StartGame)
	r.Get("/api/v1/rooms/{id}/ws", wsHandler.Handle)

	srv := &http.Server{Addr: ":" + cfg.Port, Handler: r}

	go func() {
		log.Info().Str("port", cfg.Port).Msg("alias backend starting")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("server error")
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	shutCtx, shutCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutCancel()
	srv.Shutdown(shutCtx)
	log.Info().Msg("server stopped")
}
