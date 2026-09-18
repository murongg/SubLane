package config

import (
	"fmt"
	"log/slog"
	"net"
	"strconv"
)

type Config struct {
	Addr     string
	DataDir  string
	LogLevel slog.Level
}

func Load(getenv func(string) string) (Config, error) {
	c := Config{Addr: "127.0.0.1:8080", DataDir: "./data", LogLevel: slog.LevelInfo}
	if v := getenv("SUBLANE_ADDR"); v != "" {
		c.Addr = v
	}
	if v := getenv("SUBLANE_DATA_DIR"); v != "" {
		c.DataDir = v
	}
	if v := getenv("SUBLANE_LOG_LEVEL"); v != "" {
		if err := c.LogLevel.UnmarshalText([]byte(v)); err != nil {
			return c, fmt.Errorf("SUBLANE_LOG_LEVEL: %w", err)
		}
	}
	_, port, err := net.SplitHostPort(c.Addr)
	if err != nil {
		return c, fmt.Errorf("SUBLANE_ADDR must be host:port: %w", err)
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return c, fmt.Errorf("SUBLANE_ADDR port must be between 1 and 65535")
	}
	return c, nil
}
