package main

import (
	"context"
	"errors"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/livingdolls/payment-service/internal/http/middleware"
	"github.com/livingdolls/payment-service/internal/modules/order"
	orderhttp "github.com/livingdolls/payment-service/internal/modules/order/http"
	"github.com/livingdolls/payment-service/internal/modules/order/paymenthttp"
	orderpostgres "github.com/livingdolls/payment-service/internal/modules/order/postgres"
)

func main() {
	databaseURL := os.Getenv("ORDER_DATABASE_URL")
	serviceToken := os.Getenv("ORDER_SERVICE_TOKEN")
	serverPort := os.Getenv("ORDER_HTTP_PORT")
	checkoutToken := os.Getenv("CHECKOUT_TEST_TOKEN")
	jwtSecret := os.Getenv("CUSTOMER_JWT_SECRET")
	jwtIssuer := os.Getenv("CUSTOMER_JWT_ISSUER")
	jwtAudience := os.Getenv("CUSTOMER_JWT_AUDIENCE")

	if serverPort == "" {
		serverPort = "8181"
	}

	if databaseURL == "" {
		log.Fatal("ORDER_DATABASE_URL is required")
	}

	if len(serviceToken) < 32 {
		log.Fatal("ORDER_SERVICE_TOKEN is invalid")
	}

	if len(checkoutToken) < 32 {
		log.Fatal("CHECKOUT_TEST_TOKEN is required")

	}

	if len(jwtSecret) < 32 || jwtIssuer == "" || jwtAudience == "" {
		log.Fatalf("customer JWT configuration is invalid")
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

	paymentClient, err := paymenthttp.NewClient(os.Getenv("PAYMENT_SERVICE_URL"), os.Getenv("PAYMENT_INTERNAL_TOKEN"))

	if err != nil {
		log.Fatal(err)
	}

	orderRepository := orderpostgres.NewRepository(db)
	orderConsumer := order.NewConsumer(orderRepository)

	checkoutService := order.NewCheckoutService(orderRepository, paymentClient)
	paymentFlowService := order.NewPaymentFlowService(orderRepository, paymentClient)

	orderHandler := orderhttp.NewHandler(orderConsumer, serviceToken)
	checkoutHandler := orderhttp.NewCheckoutHandler(checkoutService)
	paymentFlowHandler := orderhttp.NewPaymentFlowHandler(paymentFlowService)

	router := gin.New()

	router.Use(
		gin.Logger(),
		gin.Recovery(),
	)

	customerRoutes := router.Group(
		"/orders",
		middleware.RequireCustomerJWT(
			jwtSecret,
			jwtIssuer,
			jwtAudience,
		),
	)

	router.POST("/internal/events/payments", orderHandler.ConsumePaymentEvent)

	customerRoutes.POST("/:order_id/checkout", checkoutHandler.Start)
	customerRoutes.POST("/:order_id/pay", paymentFlowHandler.Pay)
	customerRoutes.GET("/:order_id/payment_status", paymentFlowHandler.Status)

	server := &http.Server{
		Addr:              net.JoinHostPort("127.0.0.1", serverPort),
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
			log.Printf("shutdown error: %v", err)
		}
	}()

	log.Println("Order service listening on :8181")

	err = server.ListenAndServe()

	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}
