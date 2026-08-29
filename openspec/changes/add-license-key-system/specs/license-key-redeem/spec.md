## ADDED Requirements

### Requirement: User can redeem a license key to top up Credits
The system SHALL allow authenticated users to redeem a license key by entering the raw key string. Upon successful redemption, the user's Credits balance SHALL increase atomically by the key's face value, with full audit trails.

#### Scenario: Successful redemption of unused key (hyphenated format)
- **GIVEN** a logged-in user with current Credits = C
- **WHEN** user submits a raw key string of format "XXXX-XXXX-XXXX-XXXX" that matches a canonical license_key in DB with status=unused and credits=V
- **THEN** the system normalizes input: trim whitespace, uppercase, validate, reconstruct canonical hyphenated key
- **THEN** inside a DB transaction with row lock:
  1. FOR UPDATE select the key row
  2. Assert status == unused
  3. Update key: status=used, usedBy=userId, usedAt=now
  4. Update users SET credits = credits + V, updated_at=now WHERE id=userId
  5. INSERT license_redeem_log with masked key, userId, userName, credits=V
  6. INSERT credit_log type=license_redeem, delta=+V, remark=包含keyMasked和batchName信息
- **THEN** transaction COMMIT; HTTP 200 response returns `{creditsGranted:V, newCreditsBalance:C+V}`
- **THEN** subsequent queries of the user shall show balance = C+V

#### Scenario: Successful redemption of 16-char key without hyphens (input tolerant)
- **WHEN** user pastes a 16-character alphanumeric string "abcd1234wxyz5678" (lowercase, no hyphens)
- **THEN** system SHALL normalize: uppercase + insert hyphens at positions 4-9-14 → "ABCD-1234-WXYZ-5678" → DB lookup
- **THEN** rest of redemption flow SHALL follow successful scenario identically, returning creditsGranted and new balance

#### Scenario: Format validation fails (no DB query)
- **WHEN** user submits a raw input that after normalization fails alphanumeric length=16 check OR contains invalid characters
- **THEN** system SHALL immediately return error "卡密格式不正确" (HTTP 400 / 业务错误码) without querying license_keys table

#### Scenario: Key does not exist
- **WHEN** normalized key is well formatted but has no matching row in license_keys
- **THEN** system SHALL return error "卡密不存在，请确认是否输入错误，或联系客服"

#### Scenario: Key already used (double use attempt)
- **WHEN** user submits key whose DB row has status=used
- **THEN** system SHALL return error "该卡密已被使用" and SHALL NOT increase balance
- **THEN** response MAY include optional hint usedAt timestamp for user troubleshooting

#### Scenario: Concurrent redemption of same unused key (race condition)
- **WHEN** N (>=2) simultaneous requests with same valid unused key are submitted (same user OR different users)
- **THEN** exactly 1 request SHALL succeed: key becomes used, balance incremented exactly once
- **THEN** all other N-1 requests SHALL receive deterministic error "该卡密已被使用"
- **THEN** final balance SHALL equal previous balance + V (exactly once; no double credit)

### Requirement: User can view their own redemption history
Authenticated users SHALL retrieve a paginated list of their personal redemption records.

#### Scenario: Empty history for new user
- **WHEN** user with 0 redemptions requests list
- **THEN** system SHALL return items=[], total=0, 200 OK

#### Scenario: History with records
- **WHEN** user with prior redemptions requests page=1 pageSize=20
- **THEN** system SHALL return items sorted createdAt DESC with total count
- **THEN** each row SHALL contain: keyMasked, credits, createdAt timestamp
