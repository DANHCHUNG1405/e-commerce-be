CREATE TABLE assistant.policy_chunks (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    slug text NOT NULL UNIQUE,
    title text NOT NULL,
    content text NOT NULL CHECK (char_length(content) BETWEEN 1 AND 12000),
    keywords text NOT NULL DEFAULT '',
    status text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'approved', 'archived')),
    effective_from timestamptz NOT NULL DEFAULT now(),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX assistant_policy_chunks_search_idx ON assistant.policy_chunks
USING GIN (to_tsvector('simple', title || ' ' || content || ' ' || keywords));
CREATE INDEX assistant_policy_chunks_status_effective_idx ON assistant.policy_chunks (status, effective_from);

INSERT INTO assistant.policy_chunks (slug, title, content, keywords, status) VALUES
('returns-and-exchanges', 'Chính sách mẫu: Đổi trả và đổi sản phẩm',
 'BẢN MẪU CHƯA ĐƯỢC CỬA HÀNG PHÊ DUYỆT. Điều kiện, thời hạn và chi phí đổi trả chưa được cửa hàng thiết lập trong dự án. Nếu sản phẩm bị lỗi, giao sai hoặc cần yêu cầu đổi trả, khách hàng nên liên hệ bộ phận hỗ trợ, cung cấp mã đơn hàng và thông tin mô tả/tài liệu liên quan. Việc chấp nhận yêu cầu cần được cửa hàng hoặc người bán xác nhận theo chính sách chính thức áp dụng cho sản phẩm và quy định pháp luật hiện hành. Chatbot không thể tự phê duyệt yêu cầu đổi trả.',
 'đổi trả hoàn hàng hoàn tiền sản phẩm lỗi giao sai đổi sản phẩm chính sách bảo hành', 'draft'),
('shipping-and-delivery', 'Chính sách mẫu: Vận chuyển và giao hàng',
 'BẢN MẪU CHƯA ĐƯỢC CỬA HÀNG PHÊ DUYỆT. Phí vận chuyển và thời gian giao dự kiến cần được hiển thị tại bước checkout theo địa chỉ nhận hàng và phương thức vận chuyển khả dụng. Thông tin trạng thái của một đơn cụ thể cần được kiểm tra từ dữ liệu đơn hàng/vận chuyển hiện tại. Dự án chưa cấu hình cam kết thời gian giao hàng chung; nếu chưa có dữ liệu hoặc có sai lệch, khách hàng nên liên hệ bộ phận hỗ trợ để được xác nhận.',
 'vận chuyển giao hàng phí ship phí vận chuyển thời gian giao hàng tracking theo dõi giao nhận', 'draft'),
('payments', 'Chính sách mẫu: Thanh toán',
 'BẢN MẪU CHƯA ĐƯỢC CỬA HÀNG PHÊ DUYỆT. Khách hàng nên sử dụng phương thức thanh toán được hiển thị tại checkout. Trạng thái thanh toán của đơn hàng cần căn cứ vào dữ liệu đơn hàng và xác nhận từ hệ thống thanh toán, không căn cứ vào ảnh chụp màn hình hoặc thông tin do người dùng tự khai. Nếu tài khoản đã bị trừ tiền nhưng đơn vẫn hiển thị chưa thanh toán, khách hàng nên cung cấp mã đơn cho bộ phận hỗ trợ. Không gửi mật khẩu, mã OTP, CVV hoặc thông tin xác thực thanh toán cho chatbot hay nhân viên hỗ trợ.',
 'thanh toán chuyển khoản sepay thanh toán lỗi đã trừ tiền chưa thanh toán phương thức thanh toán', 'draft'),
('cancellations', 'Chính sách mẫu: Hủy đơn hàng',
 'BẢN MẪU CHƯA ĐƯỢC CỬA HÀNG PHÊ DUYỆT. Dự án chưa thiết lập thời hạn hủy đơn áp dụng chung. Khả năng hủy có thể phụ thuộc trạng thái xử lý của đơn và chính sách chính thức của cửa hàng/người bán. Chatbot hiện chỉ hỗ trợ tra cứu, không thực hiện hoặc xác nhận hủy đơn. Khách hàng cần dùng chức năng được cung cấp trên giao diện hoặc liên hệ bộ phận hỗ trợ để kiểm tra yêu cầu cụ thể.',
 'hủy đơn hủy đơn hàng cancel cancellation hủy order', 'draft'),
('refunds', 'Chính sách mẫu: Hoàn tiền',
 'BẢN MẪU CHƯA ĐƯỢC CỬA HÀNG PHÊ DUYỆT. Dự án chưa cấu hình thời gian hoàn tiền hoặc cam kết thời điểm tiền về. Kết quả và tiến độ hoàn tiền cần được cửa hàng xác nhận theo trạng thái xử lý yêu cầu, phương thức thanh toán và xác nhận của đơn vị thanh toán. Khách hàng nên cung cấp mã đơn cho bộ phận hỗ trợ để được kiểm tra. Chatbot không thể xác nhận một khoản hoàn tiền đã được thực hiện nếu hệ thống chưa có dữ liệu tương ứng.',
 'hoàn tiền refund tiền về bao lâu hoàn trả tiền giao dịch hoàn tiền chậm', 'draft');
