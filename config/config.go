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
	Port              string `mapstructure:"port"`
	Allowedorigin     string `mapstructure:"allowedorigin"`
	StaticToken       string `mapstructure:"statictoken"`
	Sign              string `mapstructure:"sign"`
	HttpOnly          bool   `mapstructure:"httponly"`
	Secure            bool   `mapstructure:"secure"`
	Wallet            string `mapstructure:"wallet"`
	SepApiKey         string `mapstructure:"sepapikey"`
	BcsApiKey         string `mapstructure:"bcsapikey"`
	SepoliaPrivateKey string `mapstructure:"sepoliaprivatekey"`
	BNBPrivateKey     string `mapstructure:"bnbprivatekey"`
	Router            *mux.Router
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
	err := godotenv.Load()
	if err != nil {
		log.Printf("Error loading .env file: %v", err)
		return nil, err
	}

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
	httpOnlyEnv := os.Getenv("HTTP_ONLY")
	secureEnv := os.Getenv("SECURE")

	httpOnly := false
	if httpOnlyEnv != "" {
		httpOnly, _ = strconv.ParseBool(httpOnlyEnv)
	}

	secure := false
	if secureEnv != "" {
		secure, _ = strconv.ParseBool(secureEnv)
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
			Port:              os.Getenv("SERVER_PORT"),
			Allowedorigin:     os.Getenv("SERVER_ALLOWED_ORIGIN"),
			StaticToken:       os.Getenv("STATIC_TOKEN"),
			Sign:              os.Getenv("Sign"),
			HttpOnly:          httpOnly,
			Secure:            secure,
			Wallet:            os.Getenv("Wallet"),
			SepApiKey:         os.Getenv("SepApiKey"),
			BcsApiKey:         os.Getenv("BcsApiKey"),
			SepoliaPrivateKey: os.Getenv("SepoliaPrivateKey"),
			BNBPrivateKey:     os.Getenv("BNBPrivateKey"),
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
