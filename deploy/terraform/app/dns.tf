resource "aws_route53_zone" "app" {
  name    = local.domain
  comment = "Delegated from Cloudflare, records for the app and the demo webhook receiver"
}

resource "aws_route53_record" "app" {
  zone_id = aws_route53_zone.app.zone_id
  name    = local.domain
  type    = "A"
  ttl     = 300
  records = [aws_eip.app.public_ip]
}
