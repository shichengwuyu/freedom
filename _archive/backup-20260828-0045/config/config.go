package config

import (
	"crypto/rand"
	"encoding/base64"
	"log"
	"strings"

	"github.com/caarlos0/env/v11"
	"github.com/joho/godotenv"
)

type Config struct {
	Port                string `env:"PORT" envDefault:"8080"`
	AdminUsername       string `env:"ADMIN_USERNAME" envDefault:"admin"`
	AdminPassword       string `env:"ADMIN_PASSWORD" envDefault:"freedom"`
	JWTSecret           string `env:"JWT_SECRET" envDefault:"freedom"`
	JWTExpireHours      int    `env:"JWT_EXPIRE_HOURS" envDefault:"168"`
	StorageDriver       string `env:"STORAGE_DRIVER" envDefault:"mysql"`
	DatabaseDSN         string `env:"DATABASE_DSN" envDefault:"freedom:freedom123@tcp(127.0.0.1:3306)/freedom?parseTime=true&charset=utf8mb4&loc=Local"`
	PublicBaseURL       string `env:"PUBLIC_BASE_URL"`
	LinuxDoAuthorizeURL string `env:"LINUX_DO_AUTHORIZE_URL" envDefault:"https://connect.linux.do/oauth2/authorize"`
	LinuxDoTokenURL     string `env:"LINUX_DO_TOKEN_URL" envDefault:"https://connect.linux.do/oauth2/token"`
	LinuxDoUserInfoURL  string `env:"LINUX_DO_USERINFO_URL" envDefault:"https://connect.linux.do/api/user"`
	AILogDir            string `env:"AI_LOG_DIR" envDefault:"data/logs/ai-calls"`
	LicensePurchaseURL  string `env:"LICENSE_PURCHASE_URL" envDefault:"https://pay.ldxp.cn/shop/35TCHF9A"`
	LocalStorageDir     string `env:"LOCAL_STORAGE_DIR" envDefault:"data/uploads"`
	// ModelStatusURL 是模型性能指标上报地址，定时任务每 15 分钟拉取一次用于前端展示模型健康状态。
	ModelStatusURL      string `env:"MODEL_STATUS_URL" envDefault:"https://rolldek.com/api/perf-metrics/summary?hours=1"`
	// PricingURL 是上游模型定价接口地址，自动定价定时任务每 15 分钟拉取一次（2026-08-27 从每小时改为 15 分钟），
	// 按「视频 +50%、图片 +20%」的加价率换算成人民币分后写回 modelCosts，实现全自动定价。
	PricingURL          string `env:"PRICING_URL" envDefault:"https://rolldek.com/api/pricing"`
	// 对象存储（S3 兼容，如 MinIO）—— 通过环境变量注入默认 provider，部署即生效，无需手动进后台配置。
	StorageS3Endpoint   string `env:"STORAGE_S3_ENDPOINT"`
	StorageS3Region     string `env:"STORAGE_S3_REGION" envDefault:"us-east-1"`
	StorageS3Bucket     string `env:"STORAGE_S3_BUCKET"`
	StorageS3AccessKey  string `env:"STORAGE_S3_ACCESS_KEY"`
	StorageS3SecretKey  string `env:"STORAGE_S3_SECRET_KEY"`
	StorageS3PublicURL  string `env:"STORAGE_S3_PUBLIC_URL"`
	// VendorCredentialKey 用于 AES-GCM 加密供应商凭证（Cookie / AccessToken）。
	// 留空时自动生成随机密钥（dev 兜底）；生产必须配置固定密钥，否则重启后无法解密已存凭证。
	VendorCredentialKey string `env:"VENDOR_CREDENTIAL_KEY"`
}

var Cfg Config

func Load() error {
	_ = godotenv.Load()
	if err := env.Parse(&Cfg); err != nil {
		return err
	}
	if strings.TrimSpace(Cfg.JWTSecret) == "" || Cfg.JWTSecret == "freedom" || Cfg.JWTSecret == "local-dev-secret-key-change-me" {
		log.Printf("[SECURITY][WARN] JWT_SECRET 未设置或为弱默认值，临时生成 random 密钥（仅 dev 兜底）。生产部署必须在 .env 配强密钥，否则重启会清空所有 session token。")
		secret, err := randomSecret()
		if err != nil {
			return err
		}
		Cfg.JWTSecret = secret
	}
	if strings.TrimSpace(Cfg.VendorCredentialKey) == "" {
		log.Printf("[SECURITY][WARN] VENDOR_CREDENTIAL_KEY 未设置，临时生成 random 密钥（仅 dev 兜底）。生产部署必须配置固定密钥，否则重启后已加密的供应商凭证无法解密。")
		secret, err := randomSecret()
		if err != nil {
			return err
		}
		Cfg.VendorCredentialKey = secret
	}
	// 生产环境检测：如果 ADMIN_PASSWORD 仍为默认值 "freedom"，拒绝启动。
	if Cfg.AdminPassword == "freedom" {
		log.Printf("[SECURITY][WARN] ADMIN_PASSWORD 为默认值 freedom。建议生产部署设置强密码。")
	}
	return nil
}

func randomSecret() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
