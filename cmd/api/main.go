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
	reviewpostgres "github.com/livingdolls/payment-service/internal/modules/reconciliation/postgres"
	"github.com/livingdolls/payment-service/internal/modules/webhook"
	webhookpostgres "github.com/livingdolls/payment-service/internal/modules/webhook/postgres"
	"github.com/livingdolls/payment-service/internal/provider/xendit"
	"github.com/livingdolls/payment-service/internal/worker"
)

func main() {
	cfg, err := config.LoadConfig()
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	xenditProvider := xendit.NewClient(cfg.XenditSecretKey, cfg.XenditBaseURL)

	ctx := context.Background()

	db, err := database.NewPostgresPool(ctx, cfg.DatabaseURL)
	if err != nil {
		slog.Error("failed to connect postgres", "error", err)
		os.Exit(1)
	}

	defer db.Close()

	transactionManager := database.NewTransactor(db)
	webhookRepository := webhookpostgres.NewRepository(db)
	reviewRepository := reviewpostgres.NewReviewRepository(db, transactionManager)
	processPaymentUseCase := application.NewProcessPaymentUseCase(transactionManager, xenditProvider)
	createPaymentUseCase := application.NewCreatePaymentUseCase(transactionManager)
	reviewUseCase := application.NewReconciliationReviewUseCase(reviewRepository)
	webhookProcessor := application.NewWebhookProcessor(db, transactionManager)
	webhookService := webhook.NewService(webhookRepository, cfg.XenditWebhookToken)
	reconciliationProcessor := application.NewReconciliationProcessor(db, xenditProvider, xenditProvider, processPaymentUseCase)

	paymentHandler := handler.NewPaymentHandler(createPaymentUseCase, processPaymentUseCase)
	webhookHandler := handler.NewWebhookHandler(webhookService)
	reviewHandler := handler.NewReconciliationReviewHandler(reviewUseCase)

	webhookWorker := worker.NewWebhookWorker(webhookProcessor)
	reconciliationWorker := worker.NewReconciliationWorker(reconciliationProcessor)

	router := router.NewRouter(db, paymentHandler, webhookHandler, reviewHandler, cfg.AdminAPIToken, cfg.AdminAPIActor)

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

	go webhookWorker.Run(ctx)
	go reconciliationWorker.Run(ctx)

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
