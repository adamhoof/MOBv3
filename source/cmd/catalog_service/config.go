package main

import (
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/adamhoof/MOBv3/internal/config"
)

type serviceConfig struct {
	DBPath         string
	ImportEndpoint string
	HTTPAddr       string
	MaxUploadSize  int64

	MQTTBrokerURL string
	MQTTClientID  string
	MQTTTopic     string
	MQTTWorkers   int

	CAPath                string
	CatalogServerCertPath string
	CatalogServerKeyPath  string
	ClientCertPath        string
	ClientKeyPath         string

	ImportTimeout time.Duration
}

func loadConfig() (serviceConfig, error) {
	dbPath, err := config.Required("CATALOG_DB_PATH")
	if err != nil {
		return serviceConfig{}, err
	}
	mqttHost, err := config.Required("MQTT_HOST")
	if err != nil {
		return serviceConfig{}, err
	}
	mqttPort, err := config.Required("MQTT_PORT")
	if err != nil {
		return serviceConfig{}, err
	}
	mqttWorkers, err := config.OptionalInt("MQTT_WORKERS", 8)
	if err != nil {
		return serviceConfig{}, err
	}
	maxUploadSize, err := config.OptionalInt64("CATALOG_HTTP_MAX_UPLOAD_SIZE", 1<<30)
	if err != nil {
		return serviceConfig{}, err
	}
	importTimeout, err := config.OptionalDuration("CATALOG_IMPORT_TIMEOUT", time.Minute)
	if err != nil {
		return serviceConfig{}, err
	}

	host := config.Optional("CATALOG_HTTP_HOST", "0.0.0.0")
	port := config.Optional("CATALOG_HTTP_PORT", "8443")
	protocol := config.Optional("MQTT_PROTOCOL", "tcps")

	cfg := serviceConfig{
		DBPath:         dbPath,
		ImportEndpoint: config.Optional("CATALOG_IMPORT_ENDPOINT", "/catalog/import"),
		HTTPAddr:       net.JoinHostPort(host, port),
		MaxUploadSize:  maxUploadSize,
		MQTTBrokerURL:  fmt.Sprintf("%s://%s", protocol, net.JoinHostPort(mqttHost, mqttPort)),
		MQTTClientID:   config.Optional("MQTT_CATALOG_CLIENT_ID", "catalog_service"),
		MQTTTopic:      config.Optional("MQTT_TOPIC_REQUEST", "product/+/+"),
		MQTTWorkers:    mqttWorkers,
		ImportTimeout:  importTimeout,
	}

	for name, dest := range map[string]*string{
		"TLS_CA_PATH":                  &cfg.CAPath,
		"CATALOG_TLS_SERVER_CERT_PATH": &cfg.CatalogServerCertPath,
		"CATALOG_TLS_SERVER_KEY_PATH":  &cfg.CatalogServerKeyPath,
		"TLS_CLIENT_CERT_PATH":         &cfg.ClientCertPath,
		"TLS_CLIENT_KEY_PATH":          &cfg.ClientKeyPath,
	} {
		value, err := config.Required(name)
		if err != nil {
			return serviceConfig{}, err
		}
		*dest = value
	}

	if !strings.HasPrefix(cfg.ImportEndpoint, "/") {
		return serviceConfig{}, fmt.Errorf("CATALOG_IMPORT_ENDPOINT must start with /")
	}
	if cfg.MQTTWorkers <= 0 {
		return serviceConfig{}, fmt.Errorf("MQTT_WORKERS must be greater than zero")
	}
	return cfg, nil
}
