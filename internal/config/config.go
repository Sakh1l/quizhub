package config

import (
	"os"
	"strconv"
)

type Config struct {
	Port     int
	DBPath   string
	AdminPIN string
}

func Load() Config {
	port := 8080
	if s := os.Getenv("QUIZHUB_PORT"); s != "" {
		if p, err := strconv.Atoi(s); err == nil {
			port = p
		}
	}
	dbPath := os.Getenv("QUIZHUB_DB")
	if dbPath == "" {
		dbPath = "./quizhub.db"
	}
	pin := os.Getenv("QUIZHUB_ADMIN_PIN")
	if pin == "" {
		pin = "1234"
	}
	return Config{Port: port, DBPath: dbPath, AdminPIN: pin}
}
