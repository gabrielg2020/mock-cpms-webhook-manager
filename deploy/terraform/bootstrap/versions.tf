terraform {
  required_version = ">= 1.11"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 6.68"
    }
  }

  backend "s3" {
    bucket              = "mock-cpms-tfstate-455101323066"
    key                 = "bootstrap/terraform.tfstate"
    region              = "eu-west-1"
    use_lockfile        = true
    encrypt             = true
    allowed_account_ids = ["455101323066"]
  }
}

provider "aws" {
  region              = "eu-west-1"
  allowed_account_ids = ["455101323066"]

  default_tags {
    tags = {
      Project    = "mock-cpms-webhook-manager"
      ManagedBy  = "terraform"
      Repository = "github.com/gabrielg2020/mock-cpms-webhook-manager"
    }
  }
}
