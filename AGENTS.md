# Quy tắc phát triển E-commerce Backend

Tệp này là quy ước bắt buộc cho mọi thay đổi do Codex hoặc developer thực hiện trong repository.

## 1. Nguyên tắc chung

- Đọc `AGENTS.md` trước khi sửa code.
- Giữ thay đổi nhỏ, rõ ràng và tương thích với kiến trúc hiện tại.
- Không tự ý đổi module path trong `go.mod`.
- Không thêm dependency nếu có thể giải quyết bằng standard library hoặc dependency hiện có.
- Không commit file `.env`, credential, token, private key hoặc dữ liệu production.
- Không ghi secret vào log, error response, test output hoặc tài liệu.
- Không chạy thao tác phá huỷ dữ liệu nếu chưa được yêu cầu rõ ràng.

## 2. Kiến trúc

- `cmd/api` chỉ khởi tạo process, dependency và graceful shutdown.
- `internal/http` chỉ xử lý HTTP: parse request, auth context, status code và response.
- Handler không được truy cập database trực tiếp.
- Business rule đặt trong service theo domain.
- Truy cập PostgreSQL đặt trong repository, dùng GORM.
- MongoDB chỉ dùng cho document linh hoạt như product metadata; không lưu order/payment/event nghiệp vụ làm nguồn dữ liệu chính.
- PostgreSQL là source of truth cho user, seller, catalog, inventory, cart, order, payment và shipment.
- Các domain chính nằm trong `internal/modules`: `auth`, `catalog`, `cart`, `order`, `payment`, `shipping`, `review`, `outbox`.

## 3. PostgreSQL và model

- Entity public dùng UUID, không quay lại `uint` hoặc ID tự tăng.
- Tiền dùng `int64` theo đơn vị nhỏ nhất (VND hoặc cent), không dùng `float`.
- Timestamp lưu UTC.
- Email, slug, SKU và provider transaction ID phải có unique index phù hợp.
- Giá, quantity, stock và discount không được âm; validation phải tồn tại ở cả service và database constraint khi phù hợp.
- Order item luôn lưu snapshot tên sản phẩm, SKU, variant, đơn giá, số lượng và discount.
- Không xoá cứng order, payment, shipment hoặc inventory movement.
- Cập nhật inventory phải nằm trong transaction và khóa row để tránh overselling.
- `orders` là đơn tổng; marketplace phải tách thành `seller_orders` theo seller.
- Không dùng `AutoMigrate` trong production. Schema thay đổi phải dùng migration SQL versioned trong `internal/database/migrations`.
- Migration phải có số thứ tự, chạy được trên database mới, idempotency ở migration runner và không sửa migration đã chạy ở môi trường dùng chung.

## 4. Transaction và outbox

- Tạo order, trừ kho và ghi `outbox_events` phải nằm trong cùng PostgreSQL transaction.
- Dùng `outbox.Enqueue(tx, ...)` để ghi event thay vì gọi MongoDB/ClickHouse trong request transaction.
- Event phải có aggregate type/id, event type, payload JSON và timestamp.
- Consumer đọc outbox (khi được triển khai) phải có retry/backoff, idempotency và không làm mất event khi database phụ tạm thời lỗi.
- Không để lỗi `record not found` khi outbox rỗng bị log như lỗi hệ thống.
- ClickHouse chỉ nhận event bất đồng bộ, không nằm trên transaction path của order/payment.

## 5. API và bảo mật

- Version API dưới `/api/v1`.
- Mọi JSON response API dùng `internal/http/response`: `{statusCode, error, responseTimestamp, data: {msg, content}}`. HTTP status phải khớp statusCode; timestamp UTC có mili giây. Không có dữ liệu dùng content `{}`; logout trả 200. Không tự trả JSON ngoài helper này.
- Password chỉ lưu bcrypt hash; tuyệt đối không trả password trong JSON.
- JWT chỉ ký bằng secret từ environment; không hard-code secret.
- Webhook payment phải kiểm tra chữ ký và idempotency bằng provider transaction ID.
- Webhook SePay dùng `response.WebhookSuccess`: giữ envelope chung và bổ sung `success: true` ở top-level theo giao thức nhà cung cấp; các API frontend không thêm trường này. Không xác nhận thanh toán từ redirect hoặc dữ liệu frontend.
- Luôn validate input, giới hạn pagination và kiểm tra quyền seller/admin ở service layer.
- Error response không được chứa SQL, connection string hoặc stack trace production.

## 6. Go style

- Chạy `gofmt` cho mọi file Go thay đổi.
- Ưu tiên error wrapping bằng `fmt.Errorf("context: %w", err)`.
- Dùng `context.Context` cho database, MongoDB và worker I/O.
- Goroutine phải có lifecycle rõ ràng và dừng qua context khi shutdown.
- Tên package viết thường, không dùng tên chung như `util` nếu có thể đặt theo domain.
- Tránh global mutable state.
- Mọi key Redis phải được tạo bằng `internal/rediskey.Key(...)`, dùng prefix cố định `ecommerce:` để phân biệt dự án trên Redis dùng chung. Không dùng key ngoài namespace này; không dùng `FLUSHDB`/`FLUSHALL` trên Redis dùng chung.
- Chức năng realtime dùng WebSocket thuần tại `/api/v1/ws`, không dùng Socket.IO. JWT gửi trong frame auth đầu tiên, không đặt token trong URL. Event/ACK dùng envelope chung kèm event/requestId; kiểm tra quyền hội thoại ở service, không tin room/userId từ client. Mỗi connection chỉ có một data writer, hàng đợi hữu hạn, heartbeat và lifecycle theo context; frontend reconnect phải đồng bộ lịch sử theo sequence.

## 7. Kiểm tra bắt buộc

Trước khi hoàn tất thay đổi, chạy:

```powershell
gofmt -w <các-file-go-đã-sửa>
go mod tidy
go test ./...
```

Nếu thay đổi migration hoặc transaction, cần thêm integration test với PostgreSQL/MongoDB container khi có thể. Nếu không thể chạy do thiếu service, phải nêu rõ trong kết quả.

## 8. Quy trình thay đổi

- Trước khi sửa, tìm usages và kiểm tra migration/model liên quan.
- Không sửa secret trong `.env` trừ khi user yêu cầu trực tiếp.
- Khi thêm bảng hoặc field, cập nhật đồng thời model, migration, repository/service liên quan và test.
- Khi đổi public API, cập nhật README hoặc tài liệu endpoint.
- Báo cáo cuối cùng phải nêu file đã đổi, kiểm tra đã chạy và các giới hạn còn lại.
