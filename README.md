# E-commerce Backend

Backend REST API viết bằng Go, Gin, GORM, PostgreSQL và MongoDB.

## Khởi chạy nhanh

1. Sao chép `.env.example` thành `.env`.
2. Chạy PostgreSQL và API bằng `docker compose up --build`.
3. Kiểm tra API tại `http://localhost:8080/health` và trạng thái database tại `http://localhost:8080/ready`.

Để chạy không dùng Docker, cần PostgreSQL và MongoDB đang chạy (hoặc dùng các URI cloud), đặt `DATABASE_URL` và `MONGO_URL` hợp lệ trong `.env`, sau đó dùng `go run ./cmd/api`.

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

Access token có thời hạn 15 phút. Refresh token có thời hạn 30 ngày, được hash trong PostgreSQL và bị thu hồi khi logout hoặc refresh.

PostgreSQL là nguồn dữ liệu nghiệp vụ duy nhất. MongoDB chỉ dành cho document linh hoạt như `product_metadata`; `outbox_events` vẫn được giữ trong PostgreSQL và sẽ được consumer riêng chuyển tới ClickHouse khi module analytics được triển khai.
