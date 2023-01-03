package main

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type config struct {
	DockerSocks        string
	ContainerLabel     string
	Interval           time.Duration
	StartPeriod        time.Duration
	DefaultStopTimeout string
	RequestTimeout     time.Duration
	WebHookUrl         string
	WebHookKey         string
	MetricsPort        string
	MetricsEnabled     string
}

func InitConfig() *config {
	cfg := config{
		DockerSocks:        getEnv("DOCKER_SOCK", "/var/run/docker.sock"),
		ContainerLabel:     getEnv("AUTOHEAL_CONTAINER_LABEL", "all"),
		Interval:           getEnvDuration("AUTOHEAL_INTERVAL", 5),
		StartPeriod:        getEnvDuration("AUTOHEAL_START_PERIOD", 0),
		DefaultStopTimeout: getEnv("AUTOHEAL_DEFAULT_STOP_TIMEOUT", "10"),
		RequestTimeout:     getEnvDuration("CURL_TIMEOUT", 30),
		WebHookUrl:         getEnv("WEBHOOK_URL", ""),
		WebHookKey:         getEnv("WEBHOOK_KEY", "text"),
		MetricsPort:        getEnv("METRICS_PORT", "2333"),
		MetricsEnabled:     getEnv("METRICS_ENABLED", "true"),
	}

	return &cfg
}

func getEnvDuration(name string, defaultVal int) time.Duration {
	val := getEnv(name, fmt.Sprint(defaultVal))
	t, err := strconv.Atoi(val)
	if err != nil {
		t = defaultVal
	}

	return time.Duration(t) * time.Second
}

func getEnv(name string, defaultVal string) string {
	val := os.Getenv(name)
	if val == "" {
		return defaultVal
	}

	return val
}
