package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/livingdolls/payment-service/internal/application"
	"github.com/livingdolls/payment-service/internal/config"
	"github.com/livingdolls/payment-service/internal/database"
	router "github.com/livingdolls/payment-service/internal/http"
	"github.com/livingdolls/payment-service/internal/http/handler"
)

func main() {
	cfg, err := config.LoadConfig()
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	ctx := context.Background()

	db, err := database.NewPostgresPool(ctx, cfg.DatabaseURL)
	if err != nil {
		slog.Error("failed to connect postgres", "error", err)
		os.Exit(1)
	}

	defer db.Close()

	transactionManager := database.NewTransactor(db)
	createPaymentUseCase := application.NewCreatePaymentUseCase(transactionManager)
	paymentHandler := handler.NewPaymentHandler(createPaymentUseCase)

	router := router.NewRouter(db, paymentHandler)

	server := &http.Server{
		Addr:              ":" + cfg.HTTPPort,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		slog.Info("http server started", "port", cfg.HTTPPort, "env", cfg.AppEnv)

		err := server.ListenAndServe()

		if err != nil && err != http.ErrServerClosed {
			slog.Error("http server error", "error", err)

			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)

	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	<-quit

	slog.Info("shutting down http server")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		slog.Error("http server shutdown error", "error", err)
	}

	slog.Info("http server closed")
}
