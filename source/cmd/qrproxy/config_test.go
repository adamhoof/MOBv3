package main

import "testing"

func setValidConfigEnv(t *testing.T) {
	t.Helper()
	t.Setenv("PROXY_IP", "0.0.0.0")
	t.Setenv("PROXY_PORT", "9100")
	t.Setenv("USB_PRINTER", "/dev/usb/lp0")
	t.Setenv("MQTT_PROTOCOL", "tcps")
	t.Setenv("MQTT_HOST", "mosquitto_broker")
	t.Setenv("MQTT_PORT", "8883")
	t.Setenv("QRPROXY_MQTT_CLIENT_ID", "qrproxy")
	t.Setenv("QTERM_MQTT_TOPIC", "qterm/payments")
	t.Setenv("TLS_CA_PATH", "/ca.crt")
	t.Setenv("TLS_CLIENT_CERT_PATH", "/client.crt")
	t.Setenv("TLS_CLIENT_KEY_PATH", "/client.key")
}

func TestLoadConfigUsesQTermTopic(t *testing.T) {
	setValidConfigEnv(t)
	cfg, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MQTTPaymentTopic != "qterm/payments" || cfg.ProxyPort != 9100 || cfg.MQTTPort != 8883 {
		t.Fatalf("unexpected config: %+v", cfg)
	}
}

func TestLoadConfigPrefersQRProxyTopic(t *testing.T) {
	setValidConfigEnv(t)
	t.Setenv("QRPROXY_MQTT_TOPIC", "override/payments")
	cfg, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MQTTPaymentTopic != "override/payments" {
		t.Fatalf("topic=%q", cfg.MQTTPaymentTopic)
	}
}

func TestLoadConfigRejectsBadPort(t *testing.T) {
	setValidConfigEnv(t)
	t.Setenv("PROXY_PORT", "0")
	if _, err := loadConfig(); err == nil {
		t.Fatal("expected bad port error")
	}
}
