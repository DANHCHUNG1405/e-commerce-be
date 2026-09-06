# E-commerce Backend

Seller Center và tài xế nội bộ: [API và luồng tích hợp](docs/seller-driver-api.md).

Realtime chat dùng WebSocket thuần tại `/api/v1/ws`: [hướng dẫn frontend](docs/websocket.md). Cấu hình origin bằng `WEBSOCKET_ORIGINS`; không còn endpoint Socket.IO.

Mở rộng marketplace: [tiến độ và phần còn lại](docs/marketplace-roadmap.md), [cấu hình SePay QR/webhook](docs/sepay.md).

API marketplace và ví dụ request: [docs/api.md](docs/api.md).

Hướng dẫn tích hợp Frontend: [docs/frontend-api.md](docs/frontend-api.md).

Response chung: `{statusCode, error, responseTimestamp, data: {msg, content}}`.
FE đọc dữ liệu qua `data.content`, lỗi qua `data.msg`; logout trả HTTP 200 với content `{}`.

Backend REST API viết bằng Go, Gin, GORM, PostgreSQL và MongoDB.

## Khởi chạy nhanh

1. Sao chép `.env.example` thành `.env`.
2. Chạy PostgreSQL và API bằng `docker compose up --build`.
3. Kiểm tra API tại `http://localhost:8080/health` và trạng thái database tại `http://localhost:8080/ready`.

Để chạy không dùng Docker, cần PostgreSQL, MongoDB và Redis đang chạy (hoặc dùng các URI cloud), đặt `DATABASE_URL`, `MONGO_URL` và `REDIS_URL` hợp lệ trong `.env`, sau đó dùng `go run ./cmd/api`. Dùng scheme `rediss://` cho Redis TLS.

## Cấu trúc

- `cmd/api`: điểm khởi động ứng dụng
- `internal/config`: đọc và kiểm tra cấu hình
- `internal/database`: PostgreSQL, migration
- `internal/auth`: JWT và mã hoá mật khẩu
- `internal/http`: router, middleware và handlers
- `internal/models`: các entity của thương mại điện tử

### Authentication API

- `POST /api/v1/auth/register` — `{ "email", "password", "fullName" }`
- `POST /api/v1/auth/login` — `{ "email", "password" }`
- `POST /api/v1/auth/refresh` — `{ "refreshToken" }`
- `POST /api/v1/auth/logout` — `{ "refreshToken" }`
- `GET /api/v1/auth/me` — header `Authorization: Bearer <accessToken>`

Access token có thời hạn 15 phút. Refresh token có thời hạn 30 ngày, được hash và lưu trong Redis với TTL; token bị xoá khi logout hoặc refresh.

Mọi key Redis của dự án dùng prefix cố định `ecommerce:`, được tạo qua `internal/rediskey.Key(...)`. Refresh token dùng key `ecommerce:auth:refresh:<token_hash>`. Các dự án khác dùng chung Redis phải chọn namespace khác; prefix chỉ phân biệt key, không phải cơ chế phân quyền. Khi chuyển từ key cũ `auth:refresh:<token_hash>`, người dùng cần đăng nhập lại; key cũ tự hết TTL, không quét/xoá dữ liệu Redis dùng chung.

PostgreSQL là nguồn dữ liệu nghiệp vụ duy nhất. MongoDB chỉ dành cho document linh hoạt như `product_metadata`; `outbox_events` vẫn được giữ trong PostgreSQL và sẽ được consumer riêng chuyển tới ClickHouse khi module analytics được triển khai.

Sơ đồ ERD PostgreSQL được lưu tại [`docs/database.dbml`](docs/database.dbml); có thể mở file này trên dbdiagram.io.
