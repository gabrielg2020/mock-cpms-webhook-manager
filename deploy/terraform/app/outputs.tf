output "name_servers" {
  description = "Add these as NS records for uni in Cloudflare to delegate subdomain"
  value       = aws_route53_zone.app.name_servers
}

output "ecr_repository_url" {
  description = "Registry path the deploy workflow pushes images to"
  value       = aws_ecr_repository.app.repository_url
}
