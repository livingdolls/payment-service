package main

import (
	"crypto/subtle"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"sync"
)

type PaymentEvent struct {
	EventID   string `json:"event_id"`
	EventType string `json:"event_type"`
	OrderID   string `json:"order_id"`
	Amount    int64  `json:"amount"`
	Currency  string `json:"currency"`
}

func main() {
	token := os.Getenv("ORDER_SERVICE_TOKEN")

	if len(token) < 32 {
		log.Fatal("ORDER_SERVICE_TOKEN needs to be at least 32 characters")
	}

	var mu sync.Mutex

	received := make(map[string]bool)

	mux := http.NewServeMux()

	mux.HandleFunc("/internal/events/payment", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		expected := "Bearer " + token

		if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte(expected)) != 1 {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

		var event PaymentEvent

		if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if event.EventID == "" || event.EventType != "payment.captured.v1" || event.OrderID == "" || event.Amount <= 0 {
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}

		mu.Lock()
		defer mu.Unlock()

		if received[event.EventID] {
			log.Printf(
				"duplicate event ignored: %s",
				event.EventID,
			)

			w.WriteHeader(http.StatusOK)
			return
		}

		received[event.EventID] = true

		log.Printf(
			"payment event received: event=%s order=%s amount=%d",
			event.EventID,
			event.OrderID,
			event.Amount,
		)

		w.WriteHeader(http.StatusOK)
	})

	log.Println("mock order service listening on :8081")

	log.Fatal(
		http.ListenAndServe("127.0.0.1:8081", mux))
}
