## ADDED Requirements

### Requirement: Admin can batch import license keys from TXT
The admin backend SHALL allow administrators to batch import license keys by uploading a TXT file (one key per line) together with a credits face value and an optional batch name.

#### Scenario: Successful TXT import with valid keys
- **WHEN** an admin uploads a valid TXT file with 100 properly formatted keys, sets credits=20, and batchName="20面额-第一批"
- **THEN** the system SHALL parse each line and normalize: trim whitespace → uppercase → validate alphanumeric after removing hyphens length=16 → re-insert hyphens as canonical format
- **THEN** the system SHALL de-duplicate (1) within the batch using an in-memory Go map, and (2) against existing keys in license_keys table
- **THEN** the system SHALL batch-INSERT only genuinely new unused keys with assigned credits, batchName, createdBy=adminId, status=unused
- **THEN** the response SHALL return summary: `{totalLines, importedCount, duplicateCount, malformedCount, malformedSamples[]}` where malformedSamples contains first 10 bad lines for operator review

#### Scenario: Malformed and empty lines in TXT are skipped gracefully
- **WHEN** an admin uploads a TXT containing mixed valid lines, empty lines, whitespace-only lines, and invalid lines (too short / wrong chars)
- **THEN** the system SHALL import only valid normalized keys
- **THEN** empty/whitespace lines SHALL be counted separately (or folded into malformed with note)
- **THEN** malformed lines SHALL be counted; first 10 bad raw lines SHALL be returned in malformedSamples for admin review
- **THEN** the system SHALL NOT return HTTP 500 on partial malformed content (200 OK with partial counts)

#### Scenario: Invalid import parameters rejected
- **WHEN** an admin submits import with credits <= 0 OR uploaded file is 0 bytes OR file extension is not .txt / content-type not text/plain
- **THEN** the system SHALL return 4xx parameter error and SHALL NOT insert any key records

#### Scenario: Double-import same TXT reports zero imported (idempotent)
- **WHEN** an admin accidentally uploads the exact same TXT twice against the same or different batchName
- **THEN** the 2nd upload SHALL detect duplicates via license_keys unique index
- **THEN** the 2nd upload response SHALL report importedCount=0, duplicateCount=N

### Requirement: Admin can modify unused-batch credits face value
The admin backend SHALL allow an admin to adjust credits for all unused keys of a batch, as long as zero keys of that batch have status=used yet.

#### Scenario: Modify credits of a fresh unused batch
- **WHEN** an admin selects batch "20面额-第一批" (100% status=unused) and changes credits from 20 to 200 then submits
- **THEN** all unused keys of that batch SHALL be updated with new credits value
- **THEN** the response SHALL report rowsAffected = number of keys updated

#### Scenario: Reject modification after any redemption in batch
- **WHEN** an admin attempts to modify a batch where at least one key has status=used
- **THEN** the system SHALL return an error message "该批次已存在兑换记录，不可修改面额，避免对账不一致"
- **THEN** the system SHALL NOT modify any key records

### Requirement: Admin can list and filter license keys
The admin backend SHALL provide a paginated list of all license keys with filters for status, batch name, creation date range, and partial key string search.

#### Scenario: Paginated list default
- **WHEN** admin requests GET /api/admin/license-keys with page=1&pageSize=20 and no filters
- **THEN** system SHALL return up to 20 keys sorted by createdAt DESC
- **THEN** response SHALL include `{items, total}`
- **THEN** each item SHALL include: keyMasked (e.g., "ABCD-1234-****-****"), batchName, credits, status, usedBy userName (if used), usedAt timestamp, createdAt timestamp

#### Scenario: Filter by status
- **WHEN** admin adds filter status=used
- **THEN** items SHALL be limited to used keys only; usedBy + usedAt SHALL appear populated

#### Scenario: Filter by batch name exact match
- **WHEN** admin adds filter batchName="20面额-第一批"
- **THEN** items SHALL be limited to keys whose batchName matches exactly

#### Scenario: Search by partial key string (case-insensitive)
- **WHEN** admin adds keyword="ABCD"
- **THEN** items SHALL include keys whose canonical key string contains "ABCD" anywhere (normalized, case-insensitive)

### Requirement: Admin can view all redemption logs
The admin backend SHALL provide a paginated list of redemption records across all users.

#### Scenario: Paginated logs default
- **WHEN** admin requests GET /api/admin/license-redeem-logs page=1 pageSize=50
- **THEN** system SHALL return logs sorted by createdAt DESC, total count provided
- **THEN** each log entry SHALL contain: keyMasked, userId, userName, credits, createdAt

#### Scenario: Filter logs by user keyword
- **WHEN** admin adds userKeyword="alice"
- **THEN** logs SHALL be filtered to rows where userName LIKE %alice% OR userId = matched user.id if exact match found

#### Scenario: Export CSV optional (nice to have, defer)
- WHEN admin clicks "导出兑换记录CSV" button
- THEN return streaming CSV download with UTF-8 BOM headers (列: 时间、用户名、卡密、面额)
