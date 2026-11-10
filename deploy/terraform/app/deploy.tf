resource "aws_ssm_document" "deploy_app" {
  name            = "${local.name}-deploy-app"
  document_type   = "Command"
  document_format = "JSON"

  content = jsonencode({
    schemaVersion = "2.2"
    description   = "Run deploy-app, which pulls the image named in the image tag parameter and replaces the app container"
    mainSteps = [{
      action = "aws:runShellScript"
      name   = "deployApp"
      inputs = {
        runCommand     = ["/usr/local/bin/deploy-app"]
        timeoutSeconds = 120
      }
    }]
  })
}

resource "aws_iam_role" "github_deploy" {
  name                 = "${local.name}-github-deploy"
  description          = "Assumed by GitHub Actions in the production environment to roll out a published image"
  assume_role_policy   = data.aws_iam_policy_document.github_production_trust.json
  max_session_duration = 3600
}

data "aws_iam_policy_document" "github_deploy" {
  statement {
    sid = "SetImageTag"
    actions = [
      "ssm:GetParameter",
      "ssm:PutParameter",
    ]
    resources = [aws_ssm_parameter.image_tag.arn]
  }

  statement {
    sid     = "RunDeployDocumentOnAppServer"
    actions = ["ssm:SendCommand"]
    resources = [
      aws_ssm_document.deploy_app.arn,
      aws_instance.app.arn,
    ]
  }

  statement {
    sid       = "ReadCommandResults"
    actions   = ["ssm:ListCommandInvocations"]
    resources = ["*"]
  }
}

resource "aws_iam_role_policy" "github_deploy" {
  name   = "roll-out-image"
  role   = aws_iam_role.github_deploy.id
  policy = data.aws_iam_policy_document.github_deploy.json
}
