package config

import (
	"log"
	"os"

	"github.com/gorilla/mux"
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
	dir, err := os.Getwd()
	if err == nil {
		log.Printf("Current working directory: %s", dir)
	}

	configFile := "./config.yaml"

	viper.SetConfigFile(configFile)
	viper.AddConfigPath(".")
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AutomaticEnv()
	viper.SetEnvPrefix("SERVER")
	viper.SetDefault("nats.username", "user")
	viper.SetDefault("nats.password", "password")
	viper.SetDefault("server.allowedorigin", "http://localhost:3000") //cambiar por url productiva
	viper.AddConfigPath("C:/opt/docker/pokersrv/config")

	if err := viper.ReadInConfig(); err != nil {
		log.Printf("Error reading config file: %v", err)
		return nil, err
	}

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		log.Printf("Error unmarshalling config: %v", err)
		return nil, err
	}

	if cfg.Server.Allowedorigin == "" {
		cfg.Server.Allowedorigin = "http://localhost:3000"
	}

	return &cfg, nil
}

func init() {
	viper.AutomaticEnv()
}
