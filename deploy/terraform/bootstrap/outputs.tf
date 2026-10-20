output "state_bucket" {
  description = "S3 bucket that holds Terraform state for this project"
  value       = aws_s3_bucket.state.bucket
}
