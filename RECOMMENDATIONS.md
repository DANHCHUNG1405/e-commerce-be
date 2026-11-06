# Đề xuất sản phẩm theo hành vi

## Chạy local

Chạy `docker compose up --build` để khởi chạy API, một `recommendation-worker`, Redpanda, Elasticsearch và ClickHouse. Endpoint thu thập hành vi chạy ngay trong API; worker chạy ba consumer group độc lập cho enrichment, hồ sơ Redis và ClickHouse. Redis và MongoDB dùng URL trong `.env` như các service hiện có. `behavior-init` là job một lần tạo ba topic 24 partition, retention 72 giờ; môi trường production dùng nhiều broker (`KAFKA_BROKERS` là danh sách `host:port` ngăn bằng dấu phẩy) và đặt `KAFKA_REPLICATION_FACTOR` phù hợp (Compose development đặt 1). Worker áp dụng migration ClickHouse `0001_behavior_events.sql` khi khởi động. Nếu chạy qua terminal, chạy `go run ./cmd/behavior-init`, sau đó `go run ./cmd/recommendation-worker` và `go run ./cmd/api`; điền `KAFKA_BROKERS`, `CLICKHOUSE_URL`, `ELASTICSEARCH_URL`, `DATABASE_URL`, `REDIS_URL`, `JWT_SECRET` theo máy local.

Sau migration PostgreSQL, tạo lại index Elasticsearch để bổ sung `category_ids`, `review_count`, `sales`:

```powershell
docker compose run --rm --build search-reindex
```

Khi tạo/sửa sản phẩm, `categoryIds` là mảng UUID danh mục. Sản phẩm mới xuất bản cần 1–3 danh mục đang tồn tại. Draft có thể chưa có danh mục; sản phẩm đã xuất bản trước đây mà chưa có danh mục vẫn được giữ nguyên cho đến khi seller cập nhật. Quan hệ `product_categories` được ghi cùng transaction với outbox `product_search_updated`.

## API

Mọi endpoint cần access JWT, đi qua API gateway `:8080`, trả envelope `{statusCode,error,responseTimestamp,data:{msg,content}}`.

`POST /api/v1/behavior/events`: tối đa 16 KiB và 20 sự kiện/request, tối đa 120 sự kiện/phút/người dùng. `userId` luôn lấy từ JWT. `202` chỉ được trả sau khi Kafka xác nhận ghi. Ví dụ:

```json
{"events":[{"eventId":"f47621f5-f655-4e1a-9664-062cb3fbc3f9","type":"product_click","occurredAt":"2026-11-05T09:00:00Z","productId":"11111111-1111-4111-8111-111111111111","source":"home"}]}
```

Các `type` khác: `product_view` cần `durationMs` từ 5000 đến 300000; `search_submitted` cần `query` 1–200 ký tự; `recommendation_impression` cần `recommendationId` và `productIds` (1–20 UUID). Dùng UUID mới cho mỗi sự kiện và thời gian trong khoảng 10 phút trước đến 2 phút sau thời gian server. Frontend bỏ qua lỗi thu thập, không chặn hành động của người mua.

`GET /api/v1/recommendations/home?limit=12` và `GET /api/v1/products/:id/recommendations?limit=12`: `limit` từ 1 đến 20, mặc định 12. `data.content` có `recommendationId` và `products`, mỗi card gồm `id`, `sellerId`, `name`, `slug`, `imageUrl`, `minPrice`. Dùng `recommendationId` trong impression event. Không có ứng viên hợp lệ trả danh sách rỗng.

## Tích hợp frontend

- Sau khi người dùng đăng nhập, gửi `product_click` một lần khi mở trang sản phẩm (gồm `productId`, `source`). Nếu mở từ danh sách đề xuất, kèm `recommendationId` để đo CTR.
- Khi rời trang sản phẩm hoặc tab bị ẩn, cộng thời gian **chỉ khi tab hiển thị**, gửi một `product_view` nếu tổng ít nhất 5 giây; giới hạn 300 giây. Dùng `sendBeacon` hoặc `fetch` với `keepalive` và access token; với `sendBeacon` không gửi được Authorization header, nên dùng `fetch` có `keepalive`.
- Gửi `search_submitted` một lần khi người dùng thực sự submit tìm kiếm. Không phát theo mỗi lần gõ phím.
- Sau khi danh sách đề xuất xuất hiện trên màn hình, gửi một `recommendation_impression` cho các ID đã hiển thị theo lô. Dùng `IntersectionObserver` để tránh tính những card chưa thấy. Không gửi impression từng sản phẩm.
- Trong mọi trường hợp, nếu collector lỗi hoặc request bị rate limit, thao tác xem/tìm kiếm vẫn tiếp tục.

## Vận hành

Sự kiện raw/enriched giữ trong Kafka 72 giờ; ClickHouse giữ chi tiết 30 ngày qua TTL. ClickHouse dùng `ReplacingMergeTree` theo `event_id`; khi thống kê chính xác sau khi consumer ghi lại, dùng `uniqExact(event_id)` vì các bản ghi trùng ở hai partition tháng khác nhau có thể chưa được gộp. Hồ sơ Redis giữ 20 danh mục, TTL 30 ngày, điểm giảm một nửa sau 7 ngày. Cache ứng viên 5 phút, kết quả người dùng 60 giây; trước mỗi response luôn kiểm tra lại trạng thái xuất bản, seller approved và biến thể còn tồn kho trong PostgreSQL/Seller service. Không truy vấn ClickHouse trên đường đọc. Theo dõi Kafka consumer lag của `behavior-enricher-v1`, `recommendation-profile-v1`, `behavior-clickhouse-v1`, Redis memory, tốc độ insert ClickHouse, p95 API và tỷ lệ event bị DLQ. Topic DLQ chứa event không hợp lệ, chỉ dùng cho xử lý vận hành và có retention 72 giờ.

Phát hành: chạy collector và worker trước, theo dõi dữ liệu và consumer lag; sau đó bật hai vị trí gợi ý cho 10% tài khoản bằng feature flag ở frontend. So sánh CTR và tỷ lệ thêm giỏ với danh sách bán chạy trước khi mở rộng. Mục tiêu kiểm tải: 1000 sự kiện/giây trong 30 phút, burst 3000/giây trong 1 phút, 200 request đề xuất/giây; p95 đề xuất <250 ms khi cache ấm, hồ sơ cập nhật <2 phút. Cần đo trên hạ tầng thật để xác nhận. Frontend nằm ngoài repository này nên phần phát sự kiện và render cần làm trong frontend.

CLI kiểm tải dùng **access JWT của tài khoản staging** trong file local (không commit); token phải còn hiệu lực lâu hơn toàn bộ bài test. Access JWT thông thường của dự án hết hạn sau 15 phút nên không đủ cho lượt 30 phút; cần token thử nghiệm có thời hạn phù hợp trong môi trường staging. CLI không dùng refresh token. Cần ít nhất 500 token riêng biệt ở mức 1000 sự kiện/giây, 1500 token ở mức 3000/giây do rate limit theo người dùng:

```powershell
go run ./cmd/recommendation-load -url http://localhost:8080 -tokens ./staging-tokens.txt -product <published-product-uuid> -duration 30m -events 1000 -recommendations 200
go run ./cmd/recommendation-load -url http://localhost:8080 -tokens ./staging-tokens.txt -product <published-product-uuid> -duration 1m -events 3000 -recommendations 200
```

CLI in số request thành công/thất bại và p95 client cho mỗi đường. Đo Kafka lag, Redis memory và tốc độ insert ClickHouse bằng dashboard vận hành trong cùng khoảng thời gian; tránh chạy trên production nếu chưa chuẩn bị dữ liệu và dung lượng.
