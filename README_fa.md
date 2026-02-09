# FSAK - Fast Secure Awesome Kokh

[English](README.md)

FSAK یک سرور و کلاینت پروکسی SOCKS5 با عملکرد بالا و ایمن است که با زبان Go نوشته شده است. این ابزار به شما امکان می‌دهد ترافیک را به صورت ایمن بین کلاینت و سرور تونل کنید، محدودیت‌ها را دور بزنید و حریم خصوصی خود را حفظ کنید.

## ویژگی‌ها

- **عملکرد بالا**: ساخته شده با Go برای همزمانی و سرعت.
- **پشتیبانی از SOCKS5**: پشتیبانی از پروتکل استاندارد SOCKS5.
- **ترافیک امن**: امکان پشتیبانی از TLS (بسته به پیکربندی).
- **پیکربندی آسان**: پیکربندی مبتنی بر JSON.
- **چندپلتفرمی**: قابل اجرا بر روی لینوکس، ویندوز و مک‌او‌اس.

## نصب

### پیش‌نیازها

- Go 1.25+ (برای بیلد کردن از سورس)

### بیلد کردن از سورس

برای ساختن فایل‌های اجرایی کلاینت و سرور:

```bash
# کلون کردن ریپازیتوری
git clone https://github.com/paulGUZU/fsak.git
cd fsak

# ساخت کلاینت (برای سیستم عامل فعلی شما)
go build -o bin/fsak-client ./cmd/client

# ساخت سرور (برای لینوکس AMD64)
GOOS=linux GOARCH=amd64 go build -o bin/fsak-server-linux-amd64 ./cmd/server
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
  ],                         // آدرس‌های سرور (پشتیبانی از IP، CIDR و رنج)
  "host": "your-cdn-host.com", // هدر Host / SNI
  "tls": false,              // فعال‌سازی TLS
  "sni": "your-cdn-host.com", // Server Name Indication (اگر TLS فعال باشد)
  "port": 80,                // پورت شنود سرور
  "proxy_port": 1080,        // پورت SOCKS5 محلی (برای کلاینت)
  "secret": "my-secret-key"  // کلید مشترک برای احراز هویت
}
```

> [!IMPORTANT]
> **پیکربندی CDN و Cloudflare:**
> - ارتباط بین **CDN** و **سرور** شما باید از طریق **HTTP** باشد (نه HTTPS).
> - اگر از **Cloudflare** استفاده می‌کنید، باید حالت رمزنگاری SSL/TLS را روی **Flexible** تنظیم کنید.

## نحوه استفاده

### اجرای سرور

۱. یک فایل `config.json` (یا `server_config.json`) با پورت و کلید مخفی (secret) مورد نظر خود بسازید.

۲. سرور را اجرا کنید:

```bash
./bin/fsak-server -config config.json
```

![Server Screenshot](resource/img/server.png)

### اجرای کلاینت

۱. یک فایل `config.json` با آدرس‌های سرور، کلید مخفی مشترک و پورت SOCKS5 محلی مورد نظر خود بسازید.
۲. کلاینت را اجرا کنید:

```bash
./bin/fsak-client -config config.json
```

![Client Screenshot](resource/img/client.png)

۳. مرورگر یا برنامه خود را تنظیم کنید تا از پروکسی SOCKS5 در آدرس `127.0.0.1:1080` (یا هر `proxy_port` که تنظیم کرده‌اید) استفاده کند.

## لایسنس

MIT

## زنده باد ایران - به امید آزادی
