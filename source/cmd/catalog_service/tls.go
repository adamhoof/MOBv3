package main

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
)

func serverTLSConfig(cfg serviceConfig) (*tls.Config, error) {
	caCert, err := os.ReadFile(cfg.CAPath)
	if err != nil {
		return nil, fmt.Errorf("read CA cert: %w", err)
	}
	clientCAPool := x509.NewCertPool()
	if ok := clientCAPool.AppendCertsFromPEM(caCert); !ok {
		return nil, fmt.Errorf("parse CA cert")
	}
	return &tls.Config{
		ClientAuth: tls.RequireAndVerifyClientCert,
		ClientCAs:  clientCAPool,
		MinVersion: tls.VersionTLS12,
	}, nil
}

func clientTLSConfig(cfg serviceConfig) (*tls.Config, error) {
	caCert, err := os.ReadFile(cfg.CAPath)
	if err != nil {
		return nil, fmt.Errorf("read CA cert: %w", err)
	}
	rootCAs := x509.NewCertPool()
	if ok := rootCAs.AppendCertsFromPEM(caCert); !ok {
		return nil, fmt.Errorf("parse CA cert")
	}
	cert, err := tls.LoadX509KeyPair(cfg.ClientCertPath, cfg.ClientKeyPath)
	if err != nil {
		return nil, fmt.Errorf("load client cert/key: %w", err)
	}
	return &tls.Config{
		RootCAs:      rootCAs,
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}, nil
}
