package config

import "os"

type Config struct {
	HTTPAddress            string
	DatabaseURL            string
	StorageRoot            string
	PublicBaseURL          string
	BillplzAPIBaseURL      string
	BillplzAPIKey          string
	BillplzXSignatureKey   string
	BillplzCollectionID    string
	BillplzCallbackBaseURL string
}

func Load() Config {
	return Config{
		HTTPAddress:            envOrDefault("HTTP_ADDRESS", ":8080"),
		DatabaseURL:            envOrDefault("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/property_ops?sslmode=disable"),
		StorageRoot:            envOrDefault("STORAGE_ROOT", "storage"),
		PublicBaseURL:          envOrDefault("PUBLIC_BASE_URL", "http://localhost:8080"),
		BillplzAPIBaseURL:      envOrDefault("BILLPLZ_API_BASE_URL", "https://www.billplz-sandbox.com/api"),
		BillplzAPIKey:          os.Getenv("BILLPLZ_API_KEY"),
		BillplzXSignatureKey:   os.Getenv("BILLPLZ_X_SIGNATURE_KEY"),
		BillplzCollectionID:    os.Getenv("BILLPLZ_COLLECTION_ID"),
		BillplzCallbackBaseURL: envOrDefault("BILLPLZ_CALLBACK_BASE_URL", envOrDefault("PUBLIC_BASE_URL", "http://localhost:8080")),
	}
}

func envOrDefault(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}
