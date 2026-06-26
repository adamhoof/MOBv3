package main

import (
	"testing"
	"time"
)

func setValidServiceEnv(t *testing.T) {
	t.Helper()
	t.Setenv("CATALOG_DB_PATH", ":memory:")
	t.Setenv("CATALOG_HTTP_HOST", "127.0.0.1")
	t.Setenv("CATALOG_HTTP_PORT", "8443")
	t.Setenv("CATALOG_IMPORT_ENDPOINT", "/catalog/import")
	t.Setenv("CATALOG_IMPORT_TIMEOUT", "1m")
	t.Setenv("MQTT_PROTOCOL", "tcps")
	t.Setenv("MQTT_HOST", "mosquitto_broker")
	t.Setenv("MQTT_PORT", "8883")
	t.Setenv("MQTT_TOPIC_REQUEST", "product/+/+")
	t.Setenv("MQTT_WORKERS", "4")
	t.Setenv("TLS_CA_PATH", "/ca.crt")
	t.Setenv("CATALOG_TLS_SERVER_CERT_PATH", "/catalog_server.crt")
	t.Setenv("CATALOG_TLS_SERVER_KEY_PATH", "/catalog_server.key")
	t.Setenv("TLS_CLIENT_CERT_PATH", "/client.crt")
	t.Setenv("TLS_CLIENT_KEY_PATH", "/client.key")
}

func TestLoadConfigBuildsServiceConfig(t *testing.T) {
	setValidServiceEnv(t)
	cfg, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HTTPAddr != "127.0.0.1:8443" || cfg.MQTTBrokerURL != "tcps://mosquitto_broker:8883" {
		t.Fatalf("unexpected config: %+v", cfg)
	}
	if cfg.ImportTimeout != time.Minute || cfg.MQTTWorkers != 4 {
		t.Fatalf("timeout=%s workers=%d", cfg.ImportTimeout, cfg.MQTTWorkers)
	}
}

func TestLoadConfigRejectsRelativeImportEndpoint(t *testing.T) {
	setValidServiceEnv(t)
	t.Setenv("CATALOG_IMPORT_ENDPOINT", "catalog/import")
	if _, err := loadConfig(); err == nil {
		t.Fatal("expected endpoint error")
	}
}
