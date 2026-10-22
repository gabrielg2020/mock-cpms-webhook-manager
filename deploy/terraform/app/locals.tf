locals {
  name        = "mock-cpms-webhook-manager"
  domain      = "uni.gabrielg.tech"
  alert_email = "dev@gabrielg.tech"

  github_oidc_repository = "gabrielg2020@80750139/mock-cpms-webhook-manager@1418775304"

  inital_image_tag = "d4bfebb51d8977e2493f0c3cd55d5ee442afdf1b"
  caddy_image = "caddy:2.11.7-alpine@sha256:d8542f48d34a9cf4e4c11a478865229840e87e4c96ea3f439101f31a5d35f75f"
}
