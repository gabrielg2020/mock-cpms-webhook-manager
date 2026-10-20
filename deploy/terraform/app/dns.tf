resource "aws_route53_zone" "app" {
  name    = local.domain
  comment = "Delegated from Cloudflare, records for the app and the demo webhook receiver"
}
