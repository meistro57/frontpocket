package store

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"
)

type RedisClient struct {
	address string
}

func NewRedisClient(rawURL string) (*RedisClient, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid REDIS_URL: %w", err)
	}

	if parsed.Scheme != "redis" {
		return nil, fmt.Errorf("unsupported redis URL scheme: %s", parsed.Scheme)
	}

	if parsed.Host == "" {
		return nil, fmt.Errorf("redis host is required")
	}

	return &RedisClient{address: parsed.Host}, nil
}

func (c *RedisClient) Health(ctx context.Context) error {
	if c == nil || strings.TrimSpace(c.address) == "" {
		return fmt.Errorf("redis client is not configured")
	}

	dialer := &net.Dialer{Timeout: 2 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", c.address)
	if err != nil {
		return err
	}
	defer conn.Close()

	if err := conn.SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
		return err
	}

	if _, err := conn.Write([]byte("*1\r\n$4\r\nPING\r\n")); err != nil {
		return err
	}

	line, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		return err
	}

	if !strings.HasPrefix(line, "+PONG") {
		return fmt.Errorf("unexpected redis response: %q", strings.TrimSpace(line))
	}

	return nil
}
