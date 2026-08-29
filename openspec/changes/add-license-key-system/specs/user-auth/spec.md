## ADDED Requirements

### Requirement: Auth user object always exposes current Credits balance
The CurrentUser API (`GET /api/auth/me`) and successful login session response SHALL continue returning the user's numeric `credits` field so frontend balance display and optimistic update after redeem work correctly.

#### Scenario: Login returns credits in user payload
- **WHEN** user logs in successfully via username/password or OAuth
- **THEN** the returned session.user object SHALL contain `credits: number` field matching latest DB value
- **THEN** no other license/commercial fields are required (there is no isCommercial, no licenseExpireAt in pure balance model)

#### Scenario: CurrentUser /auth/me returns credits
- **WHEN** a logged-in user calls GET /api/auth/me
- **THEN** response SHALL include `credits` number value
- **THEN** existing user fields (id, username, role, displayName, email, createdAt, updatedAt) SHALL remain unchanged and backward compatible

### Requirement: Newly registered user default Credits = 0
A newly created local-account user SHALL start with Credits = 0, as the system does not grant trial credits.

#### Scenario: First signup zero balance
- **WHEN** admin creates a user OR a new user signs up (if signup is enabled)
- **THEN** the created user row SHALL have credits = 0 (or default zero value in DB)
- **THEN** no initial credit grant CreditLog entry SHALL be written
