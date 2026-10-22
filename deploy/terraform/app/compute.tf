data "aws_ssm_parameter" "al2023_arm64" {
  name = "/aws/service/ami-amazon-linux-latest/al2023-ami-kernel-default-arm64"
}

resource "aws_ssm_parameter" "image_tag" {
  name        = "/${local.name}/image_tag"
  description = "Commit SHA of the image the server runs; updated by the deploy workflow"
  type        = "String"
  value       = local.inital_image_tag

  lifecycle {
    ignore_changes = [value]
  }
}

resource "aws_instance" "app" {
  ami                         = data.aws_ssm_parameter.al2023_arm64.value
  instance_type               = "t4g.nano"
  subnet_id                   = data.aws_subnet.app.id
  vpc_security_group_ids      = [aws_security_group.app.id]
  iam_instance_profile        = aws_iam_instance_profile.app.name
  user_data_replace_on_change = true

  user_data = templatefile("${path.module}/templates/user-data.sh.tftpl", {
    region              = data.aws_region.current.region
    registry            = split("/", aws_ecr_repository.app.repository_url)[0]
    repository_url      = aws_ecr_repository.app.repository_url
    image_tag_parameter = aws_ssm_parameter.image_tag.name
    log_group           = aws_cloudwatch_log_group.app.name
    domain              = local.domain
    alert_email         = local.alert_email
    caddy_image         = local.caddy_image
    caddyfile           = chomp(file("${path.module}/../../caddy/Caddyfile"))
  })

  metadata_options {
    http_endpoint               = "enabled"
    http_tokens                 = "required"
    http_put_response_hop_limit = 1
  }

  root_block_device {
    volume_type = "gp3"
    volume_size = 8
    encrypted   = true
  }

  tags = {
    Name = local.name
  }

  lifecycle {
    ignore_changes = [ami]
  }
}

resource "aws_eip" "app" {
  domain = "vpc"

  tags = {
    Name = local.name
  }
}

resource "aws_eip_association" "app" {
  instance_id   = aws_instance.app.id
  allocation_id = aws_eip.app.id
}

data "aws_region" "current" {}
