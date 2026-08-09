# Docker Secrets Directory

This directory contains Docker secrets that are mounted into containers at `/run/secrets/`.

## Setup

**IMPORTANT: These files should contain actual secret values and must NOT be committed to version control.**

1. From the repository root, generate local-only secret files:

   ```bash
   ./scripts/secrets/init-secrets.sh
   ```

2. Edit each file with your actual secret values.

## Secret Files

| File | Description | Example |
|------|-------------|---------|
| `postgres_password.txt` | PostgreSQL app user password | Strong random password |
| `jwt_secret.txt` | Legacy signing secret for non-migrated internal services | Base64-encoded random string |
| `jwt_secret_user.txt` | User access-token signing secret | Independently generated random value |
| `jwt_refresh_secret_user.txt` | User refresh-token signing secret | Independently generated random value |
| `jwt_secret_admin.txt` | Admin access-token signing secret | Independently generated random value |
| `jwt_refresh_secret_admin.txt` | Admin refresh-token signing secret | Independently generated random value |
| `admin_mfa_encryption_key.txt` | Admin-only MFA AES-256-GCM key | 32 random bytes encoded as hexadecimal |
| `admin_mfa_recovery_pepper.txt` | Admin-only recovery-code HMAC pepper | Independent 32 random bytes encoded as hexadecimal |
| `twelvedata_api_keys.txt` | TwelveData API keys (comma-separated) | `key1,key2` |
| `massive_api_keys.txt` | Massive API keys (comma-separated) | `key1,key2` |
| `security_code_hash_secret.txt` | Dedicated HMAC key for OTP/reset digests | Independently generated random value |
| `mailerino_api_key.txt` | Mailerino security email API key for `IR` Users | Provider-issued secret |
| `resend_api_key.txt` | Resend security email API key for non-`IR` Users | Provider-issued secret |
| `kavenegar_api_key.txt` | KaveNegar SMS API key when SMS delivery is enabled | Provider-issued secret |
| `discord_webhook_url.txt` | Discord webhook URL | `https://discord.com/api/webhooks/...` |
| `jibit_api_key.txt` | Jibit payment gateway API key | `ap_xxxxxxxxx` |
| `jibit_secret_key.txt` | Jibit payment gateway secret key | `sc_xxxxxxxxx` |
| `jibit_kyc_api_key.txt` | Jibit KYC/identity API key | `ap_xxxxxxxxx` |
| `jibit_kyc_secret_key.txt` | Jibit KYC/identity secret key | `sc_xxxxxxxxx` |
| `nowpayments_api_key.txt` | NOWPayments API key | `xxxxxxxxx` |
| `nowpayments_ipn_secret.txt` | NOWPayments IPN webhook secret | `xxxxxxxxx` |
| `google_client_id.txt` | Google OAuth Client ID | `xxx.apps.googleusercontent.com` |
| `google_client_secret.txt` | Google OAuth Client Secret | `GOCSPX-xxxxxxxxx` |
| `grafana_admin_password.txt` | Grafana admin dashboard password | Strong random password |

## File Format

- Each file should contain only the secret value
- No quotes or extra formatting
- Newlines at the end are automatically trimmed
- For multiple values (API keys), use comma separation

## Example Content

**postgres_password.txt:**
```
my-secure-password-here
```

**twelvedata_api_keys.txt:**
```
your-twelvedata-key-1,your-twelvedata-key-2
```

## Security Notes

1. **Never commit actual secrets** - These files are in `.gitignore`
2. **Use strong, independent auth keys** - Generate each auth key from at least 32 random bytes; never reuse one across contexts or token purposes.
3. **Rotate regularly** - See `docs/runbook/api-key-rotation.md`
4. **Restrict file permissions** - Files should be readable only by owner
5. **Keep code hashing isolated** - Never reuse the security-code HMAC key as a JWT key or provider credential.
6. **Production requires both email routes** - Mailerino serves canonical country `IR`; Resend serves supported non-`IR` countries. Neither route falls back to the other.

## Generating Secrets

```bash
# Generate strong random password
openssl rand -base64 32 | tr -d '=+/' | head -c 32 > postgres_password.txt

# Generate the four independent User/Admin access and refresh secrets.
# Each command begins with 48 random bytes; never reuse an output.
openssl rand -base64 48 | tr -d '\n' > jwt_secret_user.txt
openssl rand -base64 48 | tr -d '\n' > jwt_refresh_secret_user.txt
openssl rand -base64 48 | tr -d '\n' > jwt_secret_admin.txt
openssl rand -base64 48 | tr -d '\n' > jwt_refresh_secret_admin.txt
openssl rand -hex 32 > admin_mfa_encryption_key.txt
openssl rand -hex 32 > admin_mfa_recovery_pepper.txt

# Generate the independent security-code HMAC key.
openssl rand -base64 48 | tr -d '\n' > security_code_hash_secret.txt
```

## Migration from `.env`

There is no automatic legacy migration. Create the four auth files with the
initializer and copy any other values manually through an approved secret
handling channel. Never copy one legacy shared JWT value into more than one of
the four isolated files.
