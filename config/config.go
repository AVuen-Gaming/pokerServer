package config

import (
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/gorilla/mux"
	"github.com/joho/godotenv"
	"github.com/spf13/viper"
)

type DatabaseConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	User     string `mapstructure:"user"`
	Password string `mapstructure:"password"`
	DBName   string `mapstructure:"dbname"`
	SSLMode  string `mapstructure:"sslmode"`
}

type NATSConfig struct {
	Host     string       `mapstructure:"host"`
	Port     int          `mapstructure:"port"`
	Username string       `mapstructure:"username"`
	Password string       `mapstructure:"password"`
	Stream   StreamConfig `mapstructure:"stream"`
}

type StreamConfig struct {
	Name     string   `mapstructure:"name"`
	Subjects []string `mapstructure:"subjects"`
}

type ServerConfig struct {
	Port          string `mapstructure:"port"`
	Allowedorigin string `mapstructure:"allowedorigin"`
	Router        *mux.Router
}

type Config struct {
	Database DatabaseConfig `mapstructure:"database"`
	NATS     NATSConfig     `mapstructure:"nats"`
	Server   ServerConfig   `mapstructure:"server"`
	Temporal TemporalConfig `mapstructure:"temporal"`
}

type TemporalConfig struct {
	HostPort string `mapstructure:"hostport"`
}

func LoadConfig() (*Config, error) {
	// Cargar el archivo .env
	err := godotenv.Load()
	if err != nil {
		log.Printf("Error loading .env file: %v", err)
		return nil, err
	}

	// Construir la configuración
	dbPort, err := strconv.Atoi(os.Getenv("DB_PORT"))
	if err != nil {
		log.Printf("Invalid database port: %v", err)
		return nil, err
	}

	natsPort, err := strconv.Atoi(os.Getenv("NATS_PORT"))
	if err != nil {
		log.Printf("Invalid NATS port: %v", err)
		return nil, err
	}

	config := &Config{
		Database: DatabaseConfig{
			Host:     os.Getenv("DB_HOST"),
			Port:     dbPort,
			User:     os.Getenv("DB_USER"),
			Password: os.Getenv("DB_PASSWORD"),
			DBName:   os.Getenv("DB_NAME"),
			SSLMode:  os.Getenv("DB_SSLMODE"),
		},
		NATS: NATSConfig{
			Host:     os.Getenv("NATS_HOST"),
			Port:     natsPort,
			Username: os.Getenv("NATS_USERNAME"),
			Password: os.Getenv("NATS_PASSWORD"),
			Stream: StreamConfig{
				Name:     os.Getenv("NATS_STREAM_NAME"),
				Subjects: strings.Split(os.Getenv("NATS_SUBJECTS"), ","),
			},
		},
		Server: ServerConfig{
			Port:          os.Getenv("SERVER_PORT"),
			Allowedorigin: os.Getenv("SERVER_ALLOWED_ORIGIN"),
		},
		Temporal: TemporalConfig{
			HostPort: os.Getenv("TEMPORAL_HOSTPORT"),
		},
	}

	return config, nil
}

func init() {
	viper.AutomaticEnv()
}
