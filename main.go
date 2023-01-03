package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/metric/instrument"
	"go.opentelemetry.io/otel/metric/instrument/syncfloat64"
	"go.opentelemetry.io/otel/sdk/metric"
)

const (
	UNIX         = "unix"
	NULL         = "null"
	RESTARTING   = "restarting"
	BASE_URL     = "http://unix/containers/"
	FILTER       = "json?filters="
	COMMAND      = "/restart?t="
	CONTENT_TYPE = "application/json"
)

type Container struct {
	Id     string            `json:"Id"`
	Names  []string          `json:"Names"`
	State  string            `json:"State"`
	Labels map[string]string `json:"Labels"`
}

type Client struct {
	httpd http.Client
	httpw http.Client
	cfg   *config
	ctr   syncfloat64.Counter
}

func NewClient() *Client {
	c := InitConfig()

	return &Client{
		cfg: c,
		httpd: http.Client{
			Timeout: c.RequestTimeout,
			Transport: &http.Transport{
				DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
					return net.Dial(UNIX, c.DockerSocks)
				},
			},
		},
		httpw: http.Client{
			Timeout: c.RequestTimeout,
		},
	}
}

func main() {
	client := NewClient()
	client.init()

	for {
		containers, err := client.getContainers()
		if err != nil {
			fmt.Printf("Failed to list containers. %s\n", err)
			continue
		}

		for _, c := range containers {
			t := time.Now().Format("2006.01.02 15:04:05")
			id := c.Id[0:12]

			if len(c.Names) == 0 || c.Names[0] == NULL {
				fmt.Printf("%s Container name of (%s) is null, which implies container does not exist - don't restart.\n", t, id)
				continue
			}

			if c.State == RESTARTING {
				fmt.Printf("%s Container %s (%s) found to be restarting - don't restart.\n", t, c.Names[0], id)
				continue
			}

			fmt.Printf("%s Container %s (%s) found to be unhealthy - Restarting container now.\n", t, c.Names[0], id)

			if err := client.restartContainer(c.Id, c.Labels["autoheal.stop.timeout"]); err != nil {
				client.addMetric(c.Names[0], "Failed to restart the container")
				if err := client.notify("%s Container %s (%s) found to be unhealthy. Failed to restart the container.\n", t, c.Names[0], id); err != nil {
					fmt.Printf("Failed to call webhook. %s\n", err)
				}
			} else {
				client.addMetric(c.Names[0], "Successfully restarted the container")
				if err := client.notify("%s Container %s (%s) found to be unhealthy. Successfully restarted the container.\n", t, c.Names[0], id); err != nil {
					fmt.Printf("Failed to call webhook. %s\n", err)
				}
			}
		}
		client.delay()
	}
}

func (c *Client) addMetric(key string, value string) {
	if c.cfg.MetricsEnabled == "true" {
		c.ctr.Add(context.TODO(), 1, []attribute.KeyValue{
			attribute.Key(key).String(value),
		}...)
	}
}

func (c *Client) serveMetrics() {
	fmt.Printf("Serving metrics at :" + c.cfg.MetricsPort + "/metrics")
	http.Handle("/metrics", promhttp.Handler())
	err := http.ListenAndServe(":"+c.cfg.MetricsPort, nil)
	if err != nil {
		log.Fatal(err)
	}
}

func (c *Client) init() {
	if c.cfg.MetricsEnabled == "true" {
		exporter, err := prometheus.New()
		if err != nil {
			log.Fatal(err)
		}
		provider := metric.NewMeterProvider(metric.WithReader(exporter))
		meter := provider.Meter("docker_restart")

		ctr, err := meter.SyncFloat64().Counter("containers_restarts", instrument.WithDescription("Total number of containers restart."))
		if err != nil {
			log.Fatal(err)
		}
		c.ctr = ctr

		go c.serveMetrics()
	}

	fmt.Printf("Monitoring containers for unhealthy status in %s\n", c.cfg.StartPeriod)
	time.Sleep(c.cfg.StartPeriod)
}

func (c *Client) delay() {
	time.Sleep(c.cfg.Interval)
}

func (c *Client) notify(format string, a ...any) error {
	fmt.Printf(format, a...)

	if c.cfg.WebHookUrl != "" {
		body, err := json.Marshal(map[string]string{c.cfg.WebHookKey: fmt.Sprintf(format, a...)})
		if err != nil {
			return err
		}

		_, err = c.httpw.Post(c.cfg.WebHookUrl, CONTENT_TYPE, bytes.NewBuffer(body))
		if err != nil {
			return err
		}
	}

	return nil
}

func (c *Client) restartContainer(id string, timeout string) error {
	t := c.cfg.DefaultStopTimeout
	if timeout != "" {
		t = timeout
	}
	_, err := c.httpd.PostForm(BASE_URL+id+COMMAND+t, url.Values{})
	return err
}

func (c *Client) getContainers() ([]Container, error) {
	qs := map[string][]string{"health": []string{"unhealthy"}}
	if c.cfg.ContainerLabel != "all" {
		qs["label"] = []string{c.cfg.ContainerLabel + "=true"}
	}
	query, err := json.Marshal(qs)

	if err != nil {
		return nil, err
	}

	response, err := c.httpd.Get(BASE_URL + FILTER + string(query[:]))
	if err != nil {
		return nil, err
	}

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}

	var containers []Container
	err = json.Unmarshal(body, &containers)
	if err != nil {
		return nil, err
	}
	return containers, nil
}
