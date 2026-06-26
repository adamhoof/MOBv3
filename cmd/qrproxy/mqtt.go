package main

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"os"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

const (
	confirmationBankPoll = "bank_poll"
	confirmationNone     = "none"
)

type qtermPaymentPayload struct {
	Amount       string `json:"amount"`
	Ref          string `json:"ref"`
	Confirmation string `json:"confirmation"`
}

type qtermPublisher struct {
	client mqtt.Client
	topic  string
}

func buildQTermPaymentPayload(amount, ref, confirmation string) ([]byte, error) {
	return json.Marshal(qtermPaymentPayload{Amount: amount, Ref: ref, Confirmation: confirmation})
}

func newQTermPublisher(cfg config) (*qtermPublisher, error) {
	tlsConfig, err := mqttTLSConfig(cfg)
	if err != nil {
		return nil, err
	}

	opts := mqtt.NewClientOptions()
	opts.AddBroker(fmt.Sprintf("%s://%s:%d", cfg.MQTTProtocol, cfg.MQTTHost, cfg.MQTTPort))
	opts.SetClientID(cfg.MQTTClientID)
	opts.SetTLSConfig(tlsConfig)
	opts.SetAutoReconnect(true)
	opts.SetConnectRetry(true)
	opts.SetCleanSession(true)
	opts.SetOrderMatters(false)

	client := mqtt.NewClient(opts)
	token := client.Connect()
	if !token.WaitTimeout(10 * time.Second) {
		return nil, fmt.Errorf("MQTT connect timed out")
	}
	if err := token.Error(); err != nil {
		return nil, fmt.Errorf("MQTT connect failed: %w", err)
	}

	return &qtermPublisher{client: client, topic: cfg.MQTTPaymentTopic}, nil
}

func mqttTLSConfig(cfg config) (*tls.Config, error) {
	cert, err := tls.LoadX509KeyPair(cfg.TLSClientCertPath, cfg.TLSClientKeyPath)
	if err != nil {
		return nil, fmt.Errorf("load MQTT client keypair: %w", err)
	}

	caBytes, err := os.ReadFile(cfg.TLSCAPath)
	if err != nil {
		return nil, fmt.Errorf("read MQTT CA: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caBytes) {
		return nil, fmt.Errorf("parse MQTT CA PEM")
	}

	return &tls.Config{
		MinVersion:   tls.VersionTLS12,
		Certificates: []tls.Certificate{cert},
		RootCAs:      pool,
	}, nil
}

func (p *qtermPublisher) publishPayment(amount, ref, confirmation string) error {
	payload, err := buildQTermPaymentPayload(amount, ref, confirmation)
	if err != nil {
		return err
	}
	token := p.client.Publish(p.topic, 1, false, payload)
	if !token.WaitTimeout(5 * time.Second) {
		return fmt.Errorf("MQTT publish timed out")
	}
	if err := token.Error(); err != nil {
		return fmt.Errorf("MQTT publish failed: %w", err)
	}
	return nil
}

func (p *qtermPublisher) close() {
	if p != nil && p.client != nil && p.client.IsConnected() {
		p.client.Disconnect(250)
	}
}
