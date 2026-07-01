package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	godiacritics "github.com/Regis24GmbH/go-diacritics"
	mqtt "github.com/eclipse/paho.mqtt.golang"

	"github.com/adamhoof/MOBv3/internal/catalog"
)

type lookupRequest struct {
	Station string
	Barcode string
}

func startProductLookupSubscriber(ctx context.Context, cfg serviceConfig, store catalog.Store) (mqtt.Client, error) {
	tlsConfig, err := clientTLSConfig(cfg)
	if err != nil {
		return nil, err
	}

	requests := make(chan lookupRequest, cfg.MQTTWorkers*4)
	clientID := cfg.MQTTClientID

	options := mqtt.NewClientOptions().
		AddBroker(cfg.MQTTBrokerURL).
		SetClientID(clientID).
		SetTLSConfig(tlsConfig).
		SetAutoReconnect(true).
		SetConnectRetry(true).
		SetConnectRetryInterval(3 * time.Second)

	var client mqtt.Client
	options.SetOnConnectHandler(func(c mqtt.Client) {
		token := c.Subscribe(cfg.MQTTTopic, 1, func(_ mqtt.Client, message mqtt.Message) {
			parts := strings.Split(message.Topic(), "/")
			if len(parts) != 3 || parts[0] != "product" || parts[1] == "" || parts[2] == "" {
				log.Printf("ignoring malformed lookup topic: %s", message.Topic())
				return
			}
			req := lookupRequest{Station: parts[1], Barcode: parts[2]}
			select {
			case requests <- req:
			default:
				log.Printf("lookup queue full, dropping barcode=%s station=%s", req.Barcode, req.Station)
			}
		})
		if token.Wait() && token.Error() != nil {
			log.Printf("subscribe failed: %s", token.Error())
			return
		}
		log.Printf("subscribed to %s", cfg.MQTTTopic)
	})

	client = mqtt.NewClient(options)
	if token := client.Connect(); !token.WaitTimeout(10 * time.Second) {
		return nil, fmt.Errorf("connect mqtt: timed out")
	} else if token.Error() != nil {
		return nil, fmt.Errorf("connect mqtt: %w", token.Error())
	}

	for workerID := 0; workerID < cfg.MQTTWorkers; workerID++ {
		go lookupWorker(ctx, workerID, client, store, requests)
	}
	return client, nil
}

func lookupWorker(ctx context.Context, workerID int, client mqtt.Client, store catalog.Store, requests <-chan lookupRequest) {
	for {
		select {
		case <-ctx.Done():
			return
		case req := <-requests:
			product, err := store.Lookup(ctx, req.Barcode)
			if err != nil {
				log.Printf("worker=%d lookup barcode=%s failed: %s", workerID, req.Barcode, err)
				product = catalog.Product{Barcode: req.Barcode, Valid: false}
			}
			product = normalizeProductResponse(product)
			payload, err := json.Marshal(product)
			if err != nil {
				log.Printf("worker=%d marshal response failed: %s", workerID, err)
				continue
			}
			responseTopic := "product/" + req.Station
			if token := client.Publish(responseTopic, 1, false, payload); token.Wait() && token.Error() != nil {
				log.Printf("worker=%d publish %s failed: %s", workerID, responseTopic, token.Error())
			}
		}
	}
}

func normalizeProductResponse(product catalog.Product) catalog.Product {
	if product.Valid {
		product.Name = godiacritics.Normalize(product.Name)
		product.Price = godiacritics.Normalize(product.Price)
	}
	return product
}
