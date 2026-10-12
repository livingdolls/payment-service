package orderservice

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/livingdolls/payment-service/internal/modules/order"
	orderhttp "github.com/livingdolls/payment-service/internal/modules/order/http"
	orderpostgres "github.com/livingdolls/payment-service/internal/modules/order/postgres"
)

func main() {
	databaseURL := os.Getenv("ORDER_DATABASE_URL")
	serviceToken := os.Getenv("ORDER_SERVICE_TOKEN")

	if databaseURL == "" {
		log.Fatal("ORDER_DATABASE_URL is required")
	}

	if len(serviceToken) < 32 {
		log.Fatal("ORDER_SERVICE_TOKEN is invalid")
	}

	ctx, stop := signal.NotifyContext(
		context.Background(),
		syscall.SIGINT,
		syscall.SIGTERM,
	)

	defer stop()

	db, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	if err := db.Ping(ctx); err != nil {
		log.Fatal(err)
	}

	orderRepository := orderpostgres.NewRepository(db)
	orderConsumer := order.NewConsumer(orderRepository)

	orderHandler := orderhttp.NewHandler(orderConsumer, serviceToken)

	router := gin.New()

	router.Use(
		gin.Logger(),
		gin.Recovery(),
	)

	router.POST("/internal/events/payments", orderHandler.ConsumePaymentEvent)

	server := &http.Server{
		Addr:              "127.0.0.1:8181",
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
	}

	go func() {
		<-ctx.Done()

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("shutdown error: %w", err)
		}
	}()

	log.Println("Order service listening on :8181")

	err = server.ListenAndServe()

	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}
