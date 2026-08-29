-- =========================================================
-- 图片破损修复：storage_objects 历史数据修正
-- 运行方式：
--   docker exec -i freedom-postgres psql -U postgres -d freedom < deploy/hk/fix-storage-objects.sql
-- 或者：
--   cd deploy/hk && docker compose exec -T postgres psql -U postgres -d freedom < fix-storage-objects.sql
-- =========================================================

-- 1. 修正旧的 Nginx /s3/freedom/ 路径为当前 MinIO 直接暴露的 :9000/freedom/ 直链
--    旧数据 public_url 形如：http://149.88.78.8/s3/freedom/2026/01/xxx.png
UPDATE storage_objects
SET public_url = regexp_replace(public_url, '^(http://[^:/]+)/s3/freedom/', '\1:9000/freedom/')
WHERE public_url ~ '^http://[^:/]+/s3/freedom/';

-- 2. 修正 https + /s3/freedom/ 形式（如果有）
UPDATE storage_objects
SET public_url = regexp_replace(public_url, '^(https://[^:/]+)/s3/freedom/', '\1:9000/freedom/')
WHERE public_url ~ '^https://[^:/]+/s3/freedom/';

-- 3. 把历史过期 storage_provider_id（storage-03418caf2b5a02aa 等不在当前 settings 里的 provider）
--    迁移到当前启用的 minio-local，按 object_key 前缀 freedom/ 匹配（MinIO bucket 的对象）
UPDATE storage_objects
SET storage_provider_id = 'minio-local'
WHERE storage_provider_id NOT IN ('minio-local')
  AND object_key LIKE 'freedom/%';

-- 4. 删除不存在的 public_url 前缀（若仍然指向旧域名 /s3/ 则再次兜底）
UPDATE storage_objects
SET public_url = regexp_replace(
    public_url,
    '^http://149\.88\.78\.8/s3/',
    'http://149.88.78.8:9000/'
)
WHERE public_url LIKE 'http://149.88.78.8/s3/%';

-- 5. 汇报本次修复数量（可选，仅输出用）
SELECT count(*) AS fixed_rows
FROM storage_objects
WHERE public_url LIKE '%:9000/freedom/%';
