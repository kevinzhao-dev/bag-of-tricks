package mpv

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

type Client struct {
	conn    net.Conn
	br      *bufio.Reader
	mu      sync.Mutex
	nextID  int
	pending map[int]chan response
	events  chan Event
	closed  chan struct{}
	closeOnce  sync.Once
	eventsOnce sync.Once
}

type response struct {
	RequestID int             `json:"request_id"`
	Error     string          `json:"error"`
	Data      json.RawMessage `json:"data"`
}

type Event struct {
	Name string
	Raw  map[string]json.RawMessage
}

func TempSocketPath() (string, func(), error) {
	dir := os.TempDir()
	name := "pp-mpv-" + strconv.FormatInt(time.Now().UnixNano(), 10) + ".sock"
	path := filepath.Join(dir, name)
	_ = os.Remove(path)
	return path, func() { _ = os.Remove(path) }, nil
}

func Dial(ctx context.Context, socketPath string) (*Client, error) {
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(5 * time.Second)
	}

	var lastErr error
	for time.Now().Before(deadline) {
		d := net.Dialer{Timeout: 250 * time.Millisecond}
		conn, err := d.DialContext(ctx, "unix", socketPath)
		if err == nil {
			c := &Client{
				conn:    conn,
				br:      bufio.NewReader(conn),
				nextID:  1,
				pending: map[int]chan response{},
				events:  make(chan Event, 128),
				closed:  make(chan struct{}),
			}
			go c.readLoop()
			return c, nil
		}
		lastErr = err
		time.Sleep(50 * time.Millisecond)
	}
	if lastErr == nil {
		lastErr = errors.New("timeout dialing mpv ipc")
	}
	return nil, lastErr
}

func (c *Client) Close() error {
	if c == nil {
		return nil
	}
	var err error
	c.closeOnce.Do(func() {
		close(c.closed)

		c.mu.Lock()
		c.pending = map[int]chan response{}
		c.mu.Unlock()

		if c.conn != nil {
			err = c.conn.Close()
		}
	})
	return err
}

func (c *Client) Events() <-chan Event { return c.events }
func (c *Client) Done() <-chan struct{} { return c.closed }

func (c *Client) readLoop() {
	defer c.eventsOnce.Do(func() { close(c.events) })
	for {
		line, err := c.br.ReadBytes('\n')
		if err != nil {
			_ = c.Close()
			return
		}

		var raw map[string]json.RawMessage
		if err := json.Unmarshal(line, &raw); err != nil {
			continue
		}

		if rid, ok := raw["request_id"]; ok {
			var id int
			if err := json.Unmarshal(rid, &id); err != nil {
				continue
			}

			var r response
			_ = json.Unmarshal(line, &r)

			c.mu.Lock()
			ch := c.pending[id]
			delete(c.pending, id)
			c.mu.Unlock()
			if ch != nil {
				ch <- r
				close(ch)
			}
			continue
		}

		if ev, ok := raw["event"]; ok {
			var name string
			if err := json.Unmarshal(ev, &name); err != nil {
				continue
			}

			e := Event{Name: name, Raw: raw}

			select {
			case c.events <- e:
			default:
			}
		}
	}
}

func (c *Client) Command(ctx context.Context, args ...any) error {
	_, err := c.CommandData(ctx, args...)
	return err
}

func (c *Client) CommandData(ctx context.Context, args ...any) (json.RawMessage, error) {
	c.mu.Lock()
	id := c.nextID
	c.nextID++
	ch := make(chan response, 1)
	c.pending[id] = ch
	c.mu.Unlock()

	req := map[string]any{
		"command":    args,
		"request_id": id,
	}
	b, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	b = append(b, '\n')
	if _, err := c.conn.Write(b); err != nil {
		return nil, err
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-c.closed:
		return nil, errors.New("mpv ipc closed")
	case r := <-ch:
		if r.Error != "success" && r.Error != "" {
			return r.Data, fmt.Errorf("mpv error: %s", r.Error)
		}
		return r.Data, nil
	}
}

func (c *Client) GetFloat(ctx context.Context, property string) (float64, error) {
	data, err := c.CommandData(ctx, "get_property", property)
	if err != nil {
		return 0, err
	}
	var f float64
	if err := json.Unmarshal(data, &f); err != nil {
		return 0, err
	}
	return f, nil
}

func (c *Client) GetBool(ctx context.Context, property string) (bool, error) {
	data, err := c.CommandData(ctx, "get_property", property)
	if err != nil {
		return false, err
	}
	var b bool
	if err := json.Unmarshal(data, &b); err != nil {
		return false, err
	}
	return b, nil
}

func (c *Client) GetString(ctx context.Context, property string) (string, error) {
	data, err := c.CommandData(ctx, "get_property", property)
	if err != nil {
		return "", err
	}
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return "", err
	}
	return s, nil
}

func (c *Client) GetInt(ctx context.Context, property string) (int, error) {
	data, err := c.CommandData(ctx, "get_property", property)
	if err != nil {
		return 0, err
	}
	var n int
	if err := json.Unmarshal(data, &n); err != nil {
		return 0, err
	}
	return n, nil
}
