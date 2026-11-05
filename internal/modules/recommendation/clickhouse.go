package recommendation

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/segmentio/kafka-go"
)

//go:embed migrations/0001_behavior_events.sql
var behaviorSchema string

type ClickHouse struct {
	URL      string
	User     string
	Password string
	Client   *http.Client
}

func (c *ClickHouse) execute(ctx context.Context, query string, body []byte) error {
	base, err := url.Parse(c.URL)
	if err != nil || (base.Scheme != "http" && base.Scheme != "https") || base.Host == "" || base.User != nil || base.RawQuery != "" || base.Fragment != "" || (base.Path != "" && base.Path != "/") {
		return fmt.Errorf("invalid ClickHouse URL")
	}
	q := base.Query()
	q.Set("query", query)
	q.Set("date_time_input_format", "best_effort")
	base.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base.String(), bytes.NewReader(body))
	if err != nil {
		return err
	}
	if c.User != "" {
		req.SetBasicAuth(c.User, c.Password)
	}
	client := c.Client
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return errors.New("ClickHouse request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		// Server error bodies may contain query text or credentials; do not expose them.
		return fmt.Errorf("ClickHouse returned status %d", resp.StatusCode)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	return nil
}

func (c *ClickHouse) Migrate(ctx context.Context) error {
	return c.execute(ctx, strings.TrimSpace(behaviorSchema), nil)
}

type ClickHouseSink struct {
	Reader     *kafka.Reader
	Store      *ClickHouse
	DeadWriter *kafka.Writer
}

func (w *ClickHouseSink) Run(ctx context.Context) error {
	batch := make([]kafka.Message, 0, 1000)
	for ctx.Err() == nil {
		waitCtx, cancel := context.WithTimeout(ctx, time.Second)
		message, err := w.Reader.FetchMessage(waitCtx)
		cancel()
		if err == nil {
			var event Event
			if decodeErr := json.Unmarshal(message.Value, &event); decodeErr != nil || event.Version != 1 || event.EventID == uuid.Nil || event.UserID == uuid.Nil || string(message.Key) != event.UserID.String() {
				if len(batch) > 0 {
					if err := w.insert(ctx, batch); err != nil {
						return err
					}
					if err := w.Reader.CommitMessages(ctx, batch...); err != nil {
						return err
					}
					batch = batch[:0]
				}
				if w.DeadWriter == nil {
					return errors.New("behavior dead letter writer unavailable")
				}
				if err := w.DeadWriter.WriteMessages(ctx, deadMessage(message, "invalid enriched ClickHouse event")); err != nil {
					return err
				}
				if err := w.Reader.CommitMessages(ctx, message); err != nil {
					return err
				}
				continue
			}
			batch = append(batch, message)
		}
		if err != nil && ctx.Err() != nil {
			break
		}
		if err != nil && !errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		if len(batch) < 1000 && err == nil {
			continue
		}
		if len(batch) == 0 {
			continue
		}
		if err := w.insert(ctx, batch); err != nil {
			return err
		}
		if err := w.Reader.CommitMessages(ctx, batch...); err != nil {
			return err
		}
		batch = batch[:0]
	}
	return ctx.Err()
}

func (w *ClickHouseSink) insert(ctx context.Context, messages []kafka.Message) error {
	var body bytes.Buffer
	encoder := json.NewEncoder(&body)
	for _, message := range messages {
		var e Event
		if err := json.Unmarshal(message.Value, &e); err != nil || e.EventID == uuid.Nil {
			return fmt.Errorf("invalid enriched behavior event at partition %d offset %d", message.Partition, message.Offset)
		}
		row := struct {
			EventID          uuid.UUID   `json:"event_id"`
			UserID           uuid.UUID   `json:"user_id"`
			Type             string      `json:"type"`
			ProductID        uuid.UUID   `json:"product_id"`
			ProductIDs       []uuid.UUID `json:"product_ids"`
			CategoryIDs      []uuid.UUID `json:"category_ids"`
			DurationMS       int64       `json:"duration_ms"`
			QueryHash        string      `json:"query_hash"`
			RecommendationID uuid.UUID   `json:"recommendation_id"`
			Source           string      `json:"source"`
			OccurredAt       time.Time   `json:"occurred_at"`
			ReceivedAt       time.Time   `json:"received_at"`
		}{e.EventID, e.UserID, e.Type, e.ProductID, e.ProductIDs, e.CategoryIDs, e.DurationMS, e.QueryHash, e.RecommendationID, e.Source, e.OccurredAt, e.ReceivedAt}
		if row.ProductIDs == nil {
			row.ProductIDs = []uuid.UUID{}
		}
		if row.CategoryIDs == nil {
			row.CategoryIDs = []uuid.UUID{}
		}
		if err := encoder.Encode(row); err != nil {
			return err
		}
	}
	return w.Store.execute(ctx, "INSERT INTO behavior_events FORMAT JSONEachRow", body.Bytes())
}
