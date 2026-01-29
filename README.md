# chin - Công Cụ Lưu Trữ & Mã Hóa Tốc Độ Cao

![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)
![Platform](https://img.shields.io/badge/Platform-Windows%20|%20Linux%20|%20macOS-lightgrey)

**chin** (Performant Independent Packer) là công cụ dòng lệnh (CLI) chuyên dụng để đóng gói và bảo mật dữ liệu, tập trung vào **an toàn bảo mật tuyệt đối** và **tốc độ xử lý tối đa**.

---

## Cài Đặt (Installation)

### 1. Cài đặt nhanh (Windows - PowerShell)
Mở PowerShell và dán lệnh sau để tự động tải bản mới nhất và cấu hình PATH:

```powershell
powershell -c "iwr -useb https://raw.githubusercontent.com/nguyendangkin/chin/main/install.ps1 | iex"
```

### 2. Cài đặt từ Source Code
Yêu cầu máy đã cài [Go](https://go.dev/dl/).

```bash
git clone https://github.com/nguyendangkin/chin.git
cd pip
go build -ldflags="-s -w" -o chin.exe .
```

---

## Hướng Dẫn Sử Dụng Chi Tiết

Công cụ có 3 lệnh chức năng: `pack`, `unpack`, và `list`.

### 1. Lệnh Đóng Gói (`pack`)

Thu thập nhiều file hoặc thư mục vào một file lưu trữ duy nhất (`.chin`).

**Cú pháp:**
```bash
chin pack [files/folders...] [flags]
```

**Các tùy chọn (Flags):**

| Flag | Viết tắt | Mặc định | Mô tả chi tiết |
| :--- | :--- | :--- | :--- |
| `--output` | `-o` | `[file_đầu].chin` | Đường dẫn file đầu ra. Nếu không nhập, lấy tên file/folder đầu tiên + đuôi `.chin`. |
| `--password` | `-p` | (Trống) | Mật khẩu mã hóa. Nếu để trống, file sẽ không được mã hóa. |
| `--split` | | (Tắt) | Kích thước tối đa mỗi phần. Hỗ trợ đơn vị **KB, MB, GB**. Không phân biệt hoa/thường. |

**Cơ chế hoạt động:**
*   **Input**: Nhận danh sách file hoặc thư mục không giới hạn số lượng.
*   **Split Naming**: Nếu dùng `--split`, file đầu tiên giữ nguyên tên (VD: `out.chin`), các file tiếp theo sẽ có đuôi `.c01`, `.c02`,... (VD: `out.chin.c01`).
*   **Progress Bar**: Hiển thị thanh tiến trình dựa trên tổng dung lượng file đầu vào.

**Ví dụ:**

```bash
# 1. Nén thư mục (Tự đặt tên file ra là 'docs.chin')
chin pack ./docs

# 2. Nén nhiều file rời rạc vào "backup.chin" có mật khẩu
chin pack -o backup.chin -p "Secret!123" file1.jpg file2.png ./data_folder

# 3. Nén và chia nhỏ mỗi file 100MB
chin pack -o game.chin --split 100MB ./GameData
# -> Kết quả: game.chin, game.chin.c01, game.chin.c02...
```

---

### 2. Lệnh Giải Nén (`unpack`)

Trích xuất dữ liệu từ file `.chin` ra ổ cứng.

**Cú pháp:**
```bash
chin unpack <archive.chin> [flags]
```

**Các tùy chọn (Flags):**

| Flag | Viết tắt | Mặc định | Mô tả chi tiết |
| :--- | :--- | :--- | :--- |
| `--destination` | `-d` | `.` (Hiện tại) | Thư mục đích để giải nén file vào. |
| `--password` | `-p` | (Trống) | Mật khẩu giải mã. Bắt buộc nếu file được mã hóa. |
| `--wrap` | | `false` | Tự động tạo thư mục chứa (Folder) dựa trên tên file nén. |

**Cơ chế hoạt động:**
*   **Wrap Logic**: Nếu bật `--wrap`:
    *   Tự tạo thư mục có tên giống file nén (VD: `data.chin` -> folder `data`).
    *   Nếu thư mục `data` đã tồn tại nhưng lại là một FILE, nó sẽ đổi tên thành `data_unpacked` để tránh lỗi.
*   **Split Joining**: Khi giải nén file chia nhỏ, chỉ cần trỏ vào file đầu tiên (`.chin`). Chương trình tự động tìm và nối các file `.c01`, `.c02`... nằm cùng thư mục.

**Ví dụ:**

```bash
# 1. Giải nén vào thư mục hiện tại
chin unpack backup.chin

# 2. Giải nén vào thư mục 'D:/Restore'
chin unpack backup.chin -d "D:/Restore"

# 3. Giải nén file có pass và tự tạo thư mục chứa
chin unpack secret.chin -p "Secret!123" --wrap
# -> Sẽ tạo thư mục 'secret' và giải nén vào đó.
```

---

### 3. Lệnh Xem Danh Sách (`list`)

Hiển thị nội dung bên trong file nén mà không giải nén.

**Cú pháp:**
```bash
chin list <archive.chin> [flags]
```

**Tùy chọn:**
*   `-p, --password`: Cần thiết nếu file metadata bị mã hóa.

**Kết quả hiển thị:**
*   **MODE**: Loại (FILE hoặc DIR).
*   **SIZE**: Kích thước file gốc (Byte).
*   **NAME**: Đường dẫn tương đối của file.

---

## Chi Tiết Kỹ Thuật & Bảo Mật

### 1. Định dạng File
*   **Mã hóa**: AES-256-GCM (Authenticated Encryption).
*   **Key Derivation (KDF)**:
    *   Sử dụng **PBKDF2-SHA256** để tạo Master Key từ mật khẩu người dùng.
    *   Sử dụng **HKDF-SHA256** để tạo khóa riêng cho TỪNG FILE (Per-File Key) từ Master Key và Salt của file đó.
*   **Chống trùng lặp Nonce (Nonce Reuse)**: Vì mỗi file có Salt riêng -> Key riêng, nên việc dùng cùng một Nonce (bộ đếm) cho nhiều file là hoàn toàn an toàn.
*   **Toàn vẹn (Integrity)**: Thuật toán GCM tự động xác thực dữ liệu khi giải mã. Nếu sai pass hoặc file bị sửa đổi, quá trình giải nén sẽ báo lỗi ngay lập tức.

### 2. Ưu điểm so với ZIP/RAR
*   **Tốc độ**: `chin` bỏ qua bước nén (compression) tốn CPU. Tốc độ nén gần như bằng tốc độ Copy file của ổ cứng. Phù hợp để lưu trữ file media (ảnh, video) vốn đã nén sẵn.
*   **Bảo mật hơn**: Zip chuẩn cũ dùng Crypto yếu. `chin` dùng chuẩn hiện đại nhất.

---

## License

MIT License.
