# E-commerce Backend

Seller Center và tài xế nội bộ: [API và luồng tích hợp](docs/seller-driver-api.md).

Realtime chat dùng WebSocket thuần tại `/api/v1/ws`: [hướng dẫn frontend](docs/websocket.md). `cmd/chat` sở hữu WebSocket và nghiệp vụ chat; API chính gọi nội bộ qua gRPC và giữ nguyên URL public. `cmd/identity` sở hữu API auth/profile/address; API chính proxy các URL public, còn Chat kiểm tra tài khoản hoạt động qua gRPC Identity. Cấu hình origin bằng `WEBSOCKET_ORIGINS`; không còn endpoint Socket.IO.

Chat Service tự chạy migration versioned trong schema `chat` và chuyển ba bảng chat được tạo bởi migration lõi cũ từ `public` sang `chat` khi khởi động. API vẫn bootstrap migration lõi trên database mới; không đổi các migration đã áp dụng.
Identity Service cũng tự chạy migration versioned trong schema `identity` cho `users`, `roles`, `user_roles` và `shipping_addresses`. Trên database mới, API hoàn tất migration lõi trước rồi chờ Identity chuyển bảng trước khi phục vụ request. PostgreSQL vẫn được dùng chung; các foreign key xuyên schema còn tồn tại.
Seller Service (`cmd/seller`) tự chuyển `sellers` và `seller_members` sang schema `seller` sau migration lõi 0010. API proxy các endpoint cửa hàng/thành viên tại URL cũ, còn dashboard, đơn hàng và tồn kho vẫn do API xử lý. Chat dùng gRPC Seller để kiểm tra trạng thái cửa hàng khi mở hội thoại. PostgreSQL vẫn dùng chung; các truy vấn và foreign key xuyên schema còn tồn tại ở các domain khác.

Mở rộng marketplace: [tiến độ và phần còn lại](docs/marketplace-roadmap.md), [cấu hình SePay QR/webhook](docs/sepay.md).

API marketplace và ví dụ request: [docs/api.md](docs/api.md).

Hướng dẫn tích hợp Frontend: [docs/frontend-api.md](docs/frontend-api.md).

Response chung: `{statusCode, error, responseTimestamp, data: {msg, content}}`.
FE đọc dữ liệu qua `data.content`, lỗi qua `data.msg`; logout trả HTTP 200 với content `{}`.

Backend REST API viết bằng Go, Gin, GORM, PostgreSQL và MongoDB.

## Khởi chạy nhanh

1. Sao chép `.env.example` thành `.env`.
2. Chạy PostgreSQL và API bằng `docker compose up --build`.
3. Kiểm tra API tại `http://localhost:8080/health`, Notification Service tại `http://localhost:8082/health`, Chat Service tại `http://localhost:8083/health`, Identity Service tại `http://localhost:8084/health`, Seller Service tại `http://localhost:8085/health` và readiness tương ứng tại `/ready`.

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
