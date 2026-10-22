output "name_servers" {
  description = "Add these as NS records for uni in Cloudflare to delegate subdomain"
  value       = aws_route53_zone.app.name_servers
}

output "ecr_repository_url" {
  description = "Registry path the deploy workflow pushes images to"
  value       = aws_ecr_repository.app.repository_url
}

output "github_publish_role_arn" {
  description = "Role the deploy workflow assumes through GitHub OIDC"
  value       = aws_iam_role.github_publish.arn
}

output "app_url" {
  description = "Public URL of the app"
  value       = "https://${local.domain}"
}

output "instance_id" {
  description = "EC2 instance for Session Manager: aws ssm start-session --target <id>"
  value       = aws_instance.app.id
}
