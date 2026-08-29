-- 本地开发用：创建 freedom 库与专用账号（与线上一致，utf8mb4）
CREATE DATABASE IF NOT EXISTS freedom CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
CREATE USER IF NOT EXISTS 'freedom'@'localhost' IDENTIFIED BY 'freedom123';
CREATE USER IF NOT EXISTS 'freedom'@'127.0.0.1' IDENTIFIED BY 'freedom123';
GRANT ALL PRIVILEGES ON freedom.* TO 'freedom'@'localhost';
GRANT ALL PRIVILEGES ON freedom.* TO 'freedom'@'127.0.0.1';
FLUSH PRIVILEGES;
SELECT '本地 freedom 库与账号已就绪' AS status;
