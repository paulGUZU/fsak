# FSAK - Fast Secure Awesome Kokh

[English](README.md)

FSAK یک سرور و کلاینت پروکسی SOCKS5 با عملکرد بالا و ایمن است که با زبان Go نوشته شده است. این ابزار به شما امکان می‌دهد ترافیک را به صورت ایمن بین کلاینت و سرور تونل کنید، محدودیت‌ها را دور بزنید و حریم خصوصی خود را حفظ کنید.

## ویژگی‌ها

- **عملکرد بالا**: ساخته شده با Go برای همزمانی و سرعت.
- **پشتیبانی از SOCKS5**: پشتیبانی از پروتکل استاندارد SOCKS5.
- **رمزنگاری AES-256-CTR**: تمام ترافیک با AES-256-CTR رمزنگاری می‌شود.
- **پروکسی سیستم**: پیکربندی خودکار پروکسی سیستم (تمام پلتفرم‌ها در حالت Proxy).
- **حالت TUN**: تونل VPN سیستم‌گسترده (فقط macOS).
- **چندپلتفرمی**: قابل اجرا بر روی Linux، Windows، macOS و FreeBSD.
- **برنامه GUI**: اپلیکیشن دسکتاپ با مدیریت پروفایل و اتصال یک‌کلیکی.
- **استخر آدرس**: متعادل‌سازی بار هوشمند بین چندین آدرس سرور.
- **پیکربندی آسان**: پیکربندی مبتنی بر JSON.

## دانلود

فایل‌های از پیش ساخته شده از [GitHub Releases](../../releases) قابل دانلود هستند.

### نسخه‌های موجود

| پلتفرم | کلاینت CLI | سرور CLI | برنامه GUI |
|----------|------------|------------|---------|
| Linux (amd64) | ✅ | ✅ | ✅ |
| Linux (arm64) | ✅ | ✅ | ❌¹ |
| Windows (amd64) | ✅ | ✅ | ✅ |
| Windows (arm64) | ✅ | ✅ | ❌² |
| macOS (amd64) | ✅ | ✅ | ✅ |
| macOS (arm64) | ✅ | ✅ | ✅ |
| FreeBSD (amd64) | ✅ | ✅ | ❌ |

> **توضیحات:**
> ¹ Linux ARM64 GUI نیاز به کتابخانه‌های پیچیده کراس‌کامپایل دارد (در CI موجود نیست).
> ² Windows ARM64 GUI مشکلات سازگاری CGO/TUN2Socks دارد.

## نصب

### گزینه ۱: دانلود فایل‌های از پیش ساخته شده

فایل مناسب برای پلتفرم خود را از [صفحه releases](../../releases) دانلود کنید.

### گزینه ۲: بیلد کردن از سورس

#### پیش‌نیازها

- Go 1.25+ (برای بیلد کردن از سورس)
- برای Linux GUI: `libgl1-mesa-dev xorg-dev`

#### بیلد کردن ابزارهای CLI

```bash
# کلون کردن ریپازیتوری
git clone https://github.com/paulGUZU/fsak.git
cd fsak

# ساخت کلاینت (برای سیستم عامل فعلی شما)
go build -o bin/fsak-client ./cmd/client

# ساخت سرور (برای سیستم عامل فعلی شما)
go build -o bin/fsak-server ./cmd/server

# نمونه کراس‌کامپایل:
# Windows AMD64
GOOS=windows GOARCH=amd64 go build -o bin/fsak-client.exe ./cmd/client

# Linux ARM64
GOOS=linux GOARCH=arm64 go build -o bin/fsak-server-linux-arm64 ./cmd/server

# macOS (یونیورسال - روی Intel و Apple Silicon اجرا می‌شود)
GOOS=darwin GOARCH=amd64 go build -o bin/fsak-client-darwin ./cmd/client
```

#### بیلد کردن GUI

```bash
# Linux (نیاز به پیش‌نیازها دارد)
# sudo apt-get install -y libgl1-mesa-dev xorg-dev
go build -o bin/fsak-gui ./cmd/gui

# macOS
go build -o bin/fsak-gui ./cmd/gui

# ساخت بسته macOS App
go install fyne.io/tools/cmd/fyne@latest
fyne package -os darwin -name FSAK -appID com.paulguzu.fsak.gui

# Windows (نیاز به mingw دارد)
go build -o bin/fsak-gui.exe ./cmd/gui
```

## پیکربندی

هر دو سمت کلاینت و سرور از یک فایل `config.json` استفاده می‌کنند.

### نمونه `config.json`

```json
{
  "addresses": [
    "1.1.1.1", 
    "2.2.2.0/24", 
    "3.3.3.3-4.4.4.4"
  ],
  "host": "your-cdn-host.com",
  "tls": false,
  "sni": "your-cdn-host.com",
  "port": 80,
  "proxy_port": 1080,
  "secret": "my-secret-key"
}
```

**فیلدهای پیکربندی:**
- `addresses`: آدرس‌های سرور (پشتیبانی از IP، رنج CIDR و رنج IP)
- `host`: هدر Host / SNI برای درخواست‌های HTTP
- `tls`: فعال‌سازی رمزنگاری TLS (نیاز به `sni` دارد)
- `sni`: Server Name Indication (اگر TLS فعال باشد الزامی است)
- `port`: پورت شنود سرور
- `proxy_port`: پورت SOCKS5 محلی (فقط کلاینت)
- `secret`: کلید مشترک برای رمزنگاری

> [!IMPORTANT]
> **پیکربندی CDN و Cloudflare:**
> - ارتباط بین **CDN** و **سرور** شما باید از طریق **HTTP** باشد (نه HTTPS).
> - اگر از **Cloudflare** استفاده می‌کنید، باید حالت رمزنگاری SSL/TLS را روی **Flexible** تنظیم کنید.

## نحوه استفاده

### اجرای سرور

۱. یک فایل `config.json` با پورت و کلید مخفی (secret) مورد نظر خود بسازید.

۲. سرور را اجرا کنید:

```bash
./bin/fsak-server -config config.json
```

![Server Screenshot](resource/img/server.png)

### اجرای کلاینت (CLI)

۱. یک فایل `config.json` با آدرس‌های سرور، کلید مخفی مشترک و پورت SOCKS5 محلی مورد نظر خود بسازید.

۲. کلاینت را اجرا کنید:

```bash
./bin/fsak-client -config config.json
```

![Client Screenshot](resource/img/client.png)

۳. کلاینت به طور خودکار پروکسی سیستم را بر روی پلتفرم‌های پشتیبانی شده (macOS، Linux، Windows) پیکربندی می‌کند.

### اجرای برنامه دسکتاپ GUI

برنامه GUI یک اپلیکیشن دسکتاپ نیتیو برای Linux، macOS و Windows است.

```bash
./bin/fsak-gui
```

**ویژگی‌های GUI:**
- **مدیریت پروفایل**: ذخیره و مدیریت چندین پروفایل اتصال
- **حالت‌های اتصال**:
  - **حالت Proxy**: پروکسی SOCKS5 با پیکربندی خودکار پروکسی سیستم (تمام پلتفرم‌ها)
  - **حالت TUN**: تونل VPN سیستم‌گسترده (فقط macOS)
- **اتصال یک‌کلیکی**: شروع/توقف آسان با نمایشگر وضعیت بصری
- **پروکسی خودکار**: پیکربندی خودکار تنظیمات پروکسی سیستم هنگام اتصال

پروفایل‌ها به صورت محلی در دایرکتوری پیکربندی سیستم عامل شما ذخیره می‌شوند:
- macOS: `~/Library/Application Support/fsak/client_profiles.json`
- Linux: `~/.config/fsak/client_profiles.json`
- Windows: `%AppData%\fsak\client_profiles.json`

### حالت‌های اتصال

#### حالت Proxy (تمام پلتفرم‌ها)
- یک پروکسی SOCKS5 محلی روی `127.0.0.1:proxy_port` اجرا می‌کند
- به طور خودکار تنظیمات پروکسی سیستم را پیکربندی می‌کند
- از macOS، Linux (GNOME/KDE) و Windows پشتیبانی می‌کند

#### حالت TUN (فقط macOS)
- یک رابط شبکه مجازی (`utun233`) ایجاد می‌کند
- تمام ترافیک سیستم را از طریق VPN هدایت می‌کند
- نیاز به دسترسی ادمین دارد

## نکات مخصوص پلتفرم‌ها

### macOS
- تمام ویژگی‌ها پشتیبانی می‌شوند
- حالت TUN نیاز به دسترسی ادمین دارد
- پروکسی سیستم از `networksetup` استفاده می‌کند

### Linux
- پروکسی سیستم از GNOME، Unity، Cinnamon، Budgie، Pantheon و KDE Plasma پشتیبانی می‌کند
- از `gsettings` یا `kwriteconfig` برای پیکربندی پروکسی استفاده می‌کند
- GUI نیاز به کتابخانه‌های توسعه OpenGL و X11 دارد

### Windows
- پروکسی سیستم از طریق Registry پیکربندی می‌شود
- به طور خودکار Windows را از تغییرات پروکسی مطلع می‌کند
- نیازی به وابستگی اضافی ندارد

### FreeBSD
- ابزارهای CLI پشتیبانی می‌شوند
- GUI در دسترس نیست (عدم پشتیبانی Fyne)

## لایسنس

MIT

## زنده باد ایران - به امید آزادی
