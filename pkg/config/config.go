package config

import (
	"os"
	"strconv"
	"time"
)

// Config contains development defaults; Docker supplies service addresses explicitly.
type Config struct {
	Service, GRPCAddr, HTTPAddr, DatabaseURL string
	UDM, AUSF, AMF, SMF, UPF, GNB            string
	RPCTimeout, RequestTimeout               time.Duration
	SeedSubscribers                          int
}

func Env(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
func Duration(name string, fallback time.Duration) time.Duration {
	value, err := time.ParseDuration(os.Getenv(name))
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}
func Load(service string) Config {
	count, err := strconv.Atoi(Env("SEED_SUBSCRIBERS", "1000"))
	if err != nil || count < 0 {
		count = 1000
	}
	return Config{Service: Env("SERVICE", service), GRPCAddr: Env("GRPC_ADDR", ":50051"), HTTPAddr: Env("HTTP_ADDR", ":8080"), DatabaseURL: os.Getenv("DATABASE_URL"),
		UDM: Env("UDM_ADDR", "localhost:50051"), AUSF: Env("AUSF_ADDR", "localhost:50052"), AMF: Env("AMF_ADDR", "localhost:50053"), SMF: Env("SMF_ADDR", "localhost:50054"), UPF: Env("UPF_ADDR", "localhost:50055"), GNB: Env("GNB_ADDR", "localhost:50056"),
		RPCTimeout: Duration("RPC_TIMEOUT", 2*time.Second), RequestTimeout: Duration("REQUEST_TIMEOUT", 5*time.Second), SeedSubscribers: count}
}
