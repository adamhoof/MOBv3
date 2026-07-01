package main

import (
	"fmt"

	envcfg "github.com/adamhoof/MOBv3/internal/config"
)

type config struct {
	ProxyIP           string
	ProxyPort         int
	USBPrinter        string
	MQTTProtocol      string
	MQTTHost          string
	MQTTPort          int
	MQTTClientID      string
	MQTTPaymentTopic  string
	TLSCAPath         string
	TLSClientCertPath string
	TLSClientKeyPath  string
}

func loadConfig() (config, error) {
	proxyIP, err := envcfg.Required("PROXY_IP")
	if err != nil {
		return config{}, err
	}
	proxyPort, err := envcfg.OptionalInt("PROXY_PORT", 0)
	if err != nil {
		return config{}, fmt.Errorf("PROXY_PORT: %w", err)
	}
	usbPrinter, err := envcfg.Required("USB_PRINTER")
	if err != nil {
		return config{}, err
	}
	mqttProtocol, err := envcfg.Required("MQTT_PROTOCOL")
	if err != nil {
		return config{}, err
	}
	mqttHost, err := envcfg.Required("MQTT_HOST")
	if err != nil {
		return config{}, err
	}
	mqttPort, err := envcfg.OptionalInt("MQTT_PORT", 0)
	if err != nil {
		return config{}, fmt.Errorf("MQTT_PORT: %w", err)
	}
	mqttClientID, err := envcfg.Required("QRPROXY_MQTT_CLIENT_ID")
	if err != nil {
		return config{}, err
	}
	topic, err := envcfg.RequiredAny("QRPROXY_MQTT_TOPIC", "QTERM_MQTT_TOPIC")
	if err != nil {
		return config{}, err
	}
	caPath, err := envcfg.Required("TLS_CA_PATH")
	if err != nil {
		return config{}, err
	}
	clientCertPath, err := envcfg.Required("TLS_CLIENT_CERT_PATH")
	if err != nil {
		return config{}, err
	}
	clientKeyPath, err := envcfg.Required("TLS_CLIENT_KEY_PATH")
	if err != nil {
		return config{}, err
	}
	if proxyPort <= 0 {
		return config{}, fmt.Errorf("PROXY_PORT must be greater than zero")
	}
	if mqttPort <= 0 {
		return config{}, fmt.Errorf("MQTT_PORT must be greater than zero")
	}

	return config{
		ProxyIP:           proxyIP,
		ProxyPort:         proxyPort,
		USBPrinter:        usbPrinter,
		MQTTProtocol:      mqttProtocol,
		MQTTHost:          mqttHost,
		MQTTPort:          mqttPort,
		MQTTClientID:      mqttClientID,
		MQTTPaymentTopic:  topic,
		TLSCAPath:         caPath,
		TLSClientCertPath: clientCertPath,
		TLSClientKeyPath:  clientKeyPath,
	}, nil
}
