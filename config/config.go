package config

import (
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
	Host   string       `mapstructure:"host"`
	Port   int          `mapstructure:"port"`
	Stream StreamConfig `mapstructure:"stream"`
}

type StreamConfig struct {
	Name     string   `mapstructure:"name"`
	Subjects []string `mapstructure:"subjects"`
}

type ServerConfig struct {
	Port string `mapstructure:"port"`
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
	viper.AddConfigPath(".")
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AutomaticEnv()
	viper.AddConfigPath("C:/opt/docker/pokersrv/config")

	if err := viper.ReadInConfig(); err != nil {
		return nil, err
	}

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func init() {
	viper.AutomaticEnv()
}
