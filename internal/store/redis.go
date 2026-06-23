package store

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type RedisClient struct {
	address  string
	password string
	db       int
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

	db := 0
	path := strings.Trim(parsed.Path, "/")
	if path != "" {
		parsedDB, convErr := strconv.Atoi(path)
		if convErr != nil {
			return nil, fmt.Errorf("invalid redis database index %q", path)
		}
		db = parsedDB
	}

	password, _ := parsed.User.Password()

	return &RedisClient{
		address:  parsed.Host,
		password: password,
		db:       db,
	}, nil
}

func (c *RedisClient) Health(ctx context.Context) error {
	reply, err := c.exec(ctx, "PING")
	if err != nil {
		return err
	}
	if !strings.EqualFold(reply.text, "PONG") {
		return fmt.Errorf("unexpected redis response: %q", reply.text)
	}
	return nil
}

func (c *RedisClient) Get(ctx context.Context, key string) (string, bool, error) {
	reply, err := c.exec(ctx, "GET", key)
	if err != nil {
		return "", false, err
	}
	if reply.nil {
		return "", false, nil
	}
	return reply.text, true, nil
}

func (c *RedisClient) SetEX(ctx context.Context, key, value string, ttl time.Duration) error {
	seconds := int(ttl.Seconds())
	if seconds <= 0 {
		seconds = 1
	}
	reply, err := c.exec(ctx, "SETEX", key, strconv.Itoa(seconds), value)
	if err != nil {
		return err
	}
	if !strings.EqualFold(reply.text, "OK") {
		return fmt.Errorf("unexpected redis response: %q", reply.text)
	}
	return nil
}

func (c *RedisClient) Del(ctx context.Context, key string) error {
	reply, err := c.exec(ctx, "DEL", key)
	if err != nil {
		return err
	}
	if _, convErr := strconv.Atoi(strings.TrimSpace(reply.text)); convErr != nil {
		return fmt.Errorf("unexpected redis DEL response: %q", reply.text)
	}
	return nil
}

type redisReply struct {
	text string
	nil  bool
}

func (c *RedisClient) exec(ctx context.Context, args ...string) (redisReply, error) {
	if c == nil || strings.TrimSpace(c.address) == "" {
		return redisReply{}, fmt.Errorf("redis client is not configured")
	}

	dialer := &net.Dialer{Timeout: 2 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", c.address)
	if err != nil {
		return redisReply{}, err
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(4 * time.Second)); err != nil {
		return redisReply{}, err
	}

	reader := bufio.NewReader(conn)

	if c.password != "" {
		if err := writeCommand(conn, "AUTH", c.password); err != nil {
			return redisReply{}, err
		}
		reply, readErr := readReply(reader)
		if readErr != nil {
			return redisReply{}, readErr
		}
		if !strings.EqualFold(reply.text, "OK") {
			return redisReply{}, fmt.Errorf("redis auth failed: %s", reply.text)
		}
	}

	if c.db > 0 {
		if err := writeCommand(conn, "SELECT", strconv.Itoa(c.db)); err != nil {
			return redisReply{}, err
		}
		reply, readErr := readReply(reader)
		if readErr != nil {
			return redisReply{}, readErr
		}
		if !strings.EqualFold(reply.text, "OK") {
			return redisReply{}, fmt.Errorf("redis select failed: %s", reply.text)
		}
	}

	if err := writeCommand(conn, args...); err != nil {
		return redisReply{}, err
	}
	return readReply(reader)
}

func writeCommand(conn net.Conn, args ...string) error {
	buf := &bytes.Buffer{}
	buf.WriteString("*")
	buf.WriteString(strconv.Itoa(len(args)))
	buf.WriteString("\r\n")
	for _, arg := range args {
		buf.WriteString("$")
		buf.WriteString(strconv.Itoa(len(arg)))
		buf.WriteString("\r\n")
		buf.WriteString(arg)
		buf.WriteString("\r\n")
	}
	_, err := conn.Write(buf.Bytes())
	return err
}

func readReply(reader *bufio.Reader) (redisReply, error) {
	prefix, err := reader.ReadByte()
	if err != nil {
		return redisReply{}, err
	}

	line, err := reader.ReadString('\n')
	if err != nil {
		return redisReply{}, err
	}
	line = strings.TrimRight(line, "\r\n")

	switch prefix {
	case '+':
		return redisReply{text: line}, nil
	case '-':
		return redisReply{}, fmt.Errorf("redis error: %s", line)
	case ':':
		return redisReply{text: line}, nil
	case '$':
		length, convErr := strconv.Atoi(line)
		if convErr != nil {
			return redisReply{}, convErr
		}
		if length < 0 {
			return redisReply{nil: true}, nil
		}
		value := make([]byte, length+2)
		if _, readErr := reader.Read(value); readErr != nil {
			return redisReply{}, readErr
		}
		return redisReply{text: string(value[:length])}, nil
	default:
		return redisReply{}, fmt.Errorf("unsupported redis reply type %q", string(prefix))
	}
}
