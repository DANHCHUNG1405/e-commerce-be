package recommendation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/example/e-commerce-be/internal/rediskey"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/segmentio/kafka-go"
)

const profileTTL = 30 * 24 * time.Hour

type Interest struct {
	CategoryID uuid.UUID `json:"categoryId"`
	Score      float64   `json:"score"`
}

type profileScore struct {
	Score float64 `json:"score"`
	At    int64   `json:"at"`
}
type profileData struct {
	Scores  map[string]profileScore `json:"scores"`
	Offsets map[string]int64        `json:"offsets"`
	Recent  []string                `json:"recent"`
}

// All state for a user is updated in one Redis script, so a replayed Kafka offset is harmless.
var updateProfileScript = redis.NewScript(`
local raw = redis.call('GET', KEYS[1])
local p = raw and cjson.decode(raw) or {scores={}, offsets={}, recent={}}
local partition = ARGV[1]
local offset = tonumber(ARGV[2])
if p.offsets[partition] and tonumber(p.offsets[partition]) >= offset then return 0 end
for _, id in ipairs(p.recent) do if id == ARGV[3] then p.offsets[partition] = offset; redis.call('SET', KEYS[1], cjson.encode(p), 'EX', 2592000); return 0 end end
local now = tonumber(ARGV[4])
local weight = tonumber(ARGV[5])
local decay = math.log(2) / 604800
for _, id in ipairs(cjson.decode(ARGV[6])) do
  local old = p.scores[id]
  local value = weight
  if old then value = tonumber(old.score) * math.exp(-decay * math.max(0, now - tonumber(old.at))) + weight end
  p.scores[id] = {score=value, at=now}
end
local ranked = {}
for id, score in pairs(p.scores) do
  local value = tonumber(score.score) * math.exp(-decay * math.max(0, now - tonumber(score.at)))
  table.insert(ranked, {id=id, score=value})
end
table.sort(ranked, function(a,b) return a.score > b.score end)
local keep = {}
for i=1, math.min(20, #ranked) do keep[ranked[i].id] = {score=ranked[i].score, at=now} end
p.scores = keep
table.insert(p.recent, ARGV[3])
if #p.recent > 256 then table.remove(p.recent, 1) end
p.offsets[partition] = offset
redis.call('SET', KEYS[1], cjson.encode(p), 'EX', 2592000)
return 1`)

type ProfileWorker struct {
	Reader     *kafka.Reader
	Redis      *redis.Client
	DeadWriter *kafka.Writer
}

func (w *ProfileWorker) Run(ctx context.Context) error {
	batch := make([]kafka.Message, 0, 100)
	for ctx.Err() == nil {
		waitCtx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
		message, err := w.Reader.FetchMessage(waitCtx)
		cancel()
		if err != nil && ctx.Err() != nil {
			break
		}
		if err != nil && !errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		if err == nil {
			var event Event
			if err := json.Unmarshal(message.Value, &event); err != nil || event.Version != 1 || event.UserID == uuid.Nil || event.EventID == uuid.Nil || string(message.Key) != event.UserID.String() {
				if len(batch) > 0 {
					if err := w.Reader.CommitMessages(ctx, batch...); err != nil {
						return err
					}
					batch = batch[:0]
				}
				if w.DeadWriter == nil {
					return errors.New("behavior dead letter writer unavailable")
				}
				if err := w.DeadWriter.WriteMessages(ctx, deadMessage(message, "invalid enriched profile event")); err != nil {
					return err
				}
				if err := w.Reader.CommitMessages(ctx, message); err != nil {
					return err
				}
				continue
			}
			if err := w.Apply(ctx, event, message.Partition, message.Offset); err != nil {
				return err
			}
			batch = append(batch, message)
		}
		if len(batch) >= 100 || (err != nil && len(batch) > 0) {
			if err := w.Reader.CommitMessages(ctx, batch...); err != nil {
				return err
			}
			batch = batch[:0]
		}
	}
	return ctx.Err()
}

func (w *ProfileWorker) Apply(ctx context.Context, event Event, partition int, offset int64) error {
	if event.Weight() == 0 || len(event.CategoryIDs) == 0 {
		return nil
	}
	ids := make([]string, 0, len(event.CategoryIDs))
	for _, id := range event.CategoryIDs {
		if id != uuid.Nil {
			ids = append(ids, id.String())
		}
	}
	body, _ := json.Marshal(ids)
	key := rediskey.Key("recommendation", "profile", event.UserID.String())
	return updateProfileScript.Run(ctx, w.Redis, []string{key}, partition, offset, event.EventID.String(), event.ReceivedAt.Unix(), event.Weight(), string(body)).Err()
}

func LoadInterests(ctx context.Context, client *redis.Client, userID uuid.UUID, now time.Time) ([]Interest, error) {
	if client == nil {
		return nil, fmt.Errorf("Redis unavailable")
	}
	raw, err := client.Get(ctx, rediskey.Key("recommendation", "profile", userID.String())).Bytes()
	if err == redis.Nil {
		return []Interest{}, nil
	}
	if err != nil {
		return nil, err
	}
	var profile profileData
	if err := json.Unmarshal(raw, &profile); err != nil {
		return nil, err
	}
	items := make([]Interest, 0, len(profile.Scores))
	for rawID, value := range profile.Scores {
		id, err := uuid.Parse(rawID)
		if err != nil {
			continue
		}
		age := math.Max(0, now.Sub(time.Unix(value.At, 0)).Seconds())
		items = append(items, Interest{CategoryID: id, Score: value.Score * math.Exp(-math.Ln2*age/(7*24*3600))})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Score > items[j].Score })
	return items, nil
}
