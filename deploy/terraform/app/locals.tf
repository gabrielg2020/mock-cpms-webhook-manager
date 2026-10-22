locals {
  name        = "mock-cpms-webhook-manager"
  domain      = "uni.gabrielg.tech"
  alert_email = "dev@gabrielg.tech"

  github_oidc_repository = "gabrielg2020@80750139/mock-cpms-webhook-manager@1418775304"

  initial_image_tag = "d4bfebb51d8977e2493f0c3cd55d5ee442afdf1b"
  caddy_image       = trimspace(trimprefix(file("${path.module}/../../caddy/Dockerfile"), "FROM "))
}
