package main

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

func parseQRPayArgs(arg string) (amount, ref string) {
	var key string
	for _, part := range strings.Fields(arg) {
		switch {
		case strings.HasPrefix(part, "A:"):
			key = "A"
			amount = part[2:]
		case strings.HasPrefix(part, "VS:"):
			key = "VS"
			ref = part[3:]
		default:
			switch key {
			case "A":
				amount = part
			case "VS":
				ref = part
			}
		}
	}
	return
}

func parseCommand(data []byte) (cmd, arg string, stripped []byte) {
	prefix := []byte("CMD:")
	idx := bytes.Index(data, prefix)
	if idx < 0 {
		return "", "", data
	}

	lineEnd := bytes.IndexByte(data[idx:], '\n')
	var cmdLine []byte
	var rest []byte
	if lineEnd >= 0 {
		cmdLine = data[idx+len(prefix) : idx+lineEnd]
		rest = append(data[:idx:idx], data[idx+lineEnd+1:]...)
	} else {
		cmdLine = data[idx+len(prefix):]
		rest = data[:idx]
	}

	s := strings.TrimSpace(string(cmdLine))
	if i := strings.IndexByte(s, ' '); i >= 0 {
		return s[:i], strings.TrimSpace(s[i+1:]), rest
	}
	return s, "", rest
}

func runSocketListener(ctx context.Context, ln net.Listener, receiptCh chan<- []byte) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			slog.Error("accept error", "err", err)
			continue
		}

		if err := conn.SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
			slog.Warn("connection deadline failed", "err", err)
		}

		var data []byte
		buf := make([]byte, 4096)
		for {
			n, err := conn.Read(buf)
			if n > 0 {
				data = append(data, buf[:n]...)
			}
			if err != nil {
				break
			}
		}
		conn.Close()

		if len(data) > 0 {
			receiptCh <- data
		}
	}
}

func main() {
	cfg, err := loadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		os.Exit(1)
	}

	level := slog.LevelInfo
	if os.Getenv("DEBUG") == "1" {
		level = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})))

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	publisher, err := newQTermPublisher(cfg)
	if err != nil {
		slog.Error("MQTT publisher init failed", "err", err)
		os.Exit(1)
	}
	defer publisher.close()

	receiptCh := make(chan []byte, 16)
	printerCh := make(chan []byte, 8)
	go runPrinter(ctx, cfg, printerCh)

	addr := fmt.Sprintf("%s:%d", cfg.ProxyIP, cfg.ProxyPort)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		slog.Error("listen error", "err", err)
		os.Exit(1)
	}
	slog.Info("QRProxy ready", "addr", addr, "mqtt_host", cfg.MQTTHost, "mqtt_topic", cfg.MQTTPaymentTopic)

	go func() {
		<-ctx.Done()
		ln.Close()
	}()
	go runSocketListener(ctx, ln, receiptCh)

	for {
		select {
		case <-ctx.Done():
			slog.Info("shutdown complete")
			return
		case raw := <-receiptCh:
			cmd, arg, printData := parseCommand(raw)
			switch cmd {
			case "QR_PAY":
				publishPaymentCommand(publisher, arg, confirmationBankPoll)
			case "RECIEPT":
				printerCh <- printData
				publishPaymentCommand(publisher, arg, confirmationNone)
			case "RESTART":
				slog.Info("restart command received")
				os.Exit(0)
			default:
				printerCh <- printData
			}
		}
	}
}

func publishPaymentCommand(publisher *qtermPublisher, arg, confirmation string) {
	amount, ref := parseQRPayArgs(arg)
	if amount == "" {
		slog.Warn("payment command missing amount")
		return
	}
	amount = strings.ReplaceAll(amount, ",", ".")
	if err := publisher.publishPayment(amount, ref, confirmation); err != nil {
		slog.Error("QTerm payment publish failed", "err", err)
		return
	}
	slog.Info("QTerm payment published", "amount", amount, "ref", ref, "confirmation", confirmation)
}
