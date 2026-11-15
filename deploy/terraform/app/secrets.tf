# Terraform generates these values and writes them straight to Parameter Store.
# Ephemeral resources are never saved, so the secrets are in neither the state
# file nor a plan. To replace a value, raise its value_wo_version.

ephemeral "random_password" "seed_admin" {
  length  = 24
  special = false
}

ephemeral "random_password" "seed_member" {
  length  = 24
  special = false
}

ephemeral "random_bytes" "secret_encryption_key" {
  length = 32
}

resource "aws_ssm_parameter" "seed_admin_password" {
  name             = "/${local.name}/seed_admin_password"
  description      = "Password the demo admin account is given when an empty database is seeded"
  type             = "SecureString"
  value_wo         = ephemeral.random_password.seed_admin.result
  value_wo_version = 1
}

resource "aws_ssm_parameter" "seed_member_password" {
  name             = "/${local.name}/seed_member_password"
  description      = "Password the demo member account is given when an empty database is seeded"
  type             = "SecureString"
  value_wo         = ephemeral.random_password.seed_member.result
  value_wo_version = 1
}

resource "aws_ssm_parameter" "secret_encryption_key" {
  name             = "/${local.name}/secret_encryption_key"
  description      = "AES-256 key, base64 encoded, that encrypts webhook signing secrets in the database"
  type             = "SecureString"
  value_wo         = ephemeral.random_bytes.secret_encryption_key.base64
  value_wo_version = 1
}
