# E-commerce Backend

Backend REST API viết bằng Go, Gin, GORM và PostgreSQL.

## Khởi chạy nhanh

1. Sao chép `.env.example` thành `.env`.
2. Chạy PostgreSQL và API bằng `docker compose up --build`.
3. Kiểm tra API tại `http://localhost:8080/health` và trạng thái database tại `http://localhost:8080/ready`.

Để chạy không dùng Docker, cần một PostgreSQL đang chạy và `DATABASE_URL` hợp lệ trong `.env`, sau đó dùng `go run ./cmd/api`.

## Cấu trúc

- `cmd/api`: điểm khởi động ứng dụng
- `internal/config`: đọc và kiểm tra cấu hình
- `internal/database`: PostgreSQL, migration
- `internal/auth`: JWT và mã hoá mật khẩu
- `internal/http`: router, middleware và handlers
- `internal/models`: các entity của thương mại điện tử

Các endpoint nghiệp vụ (auth, catalog, cart, order) sẽ được thêm trong `internal/http` và service/repository tương ứng.
