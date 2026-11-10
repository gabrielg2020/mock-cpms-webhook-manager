#!/bin/bash
# Points the server at an image tag and runs deploy-app on it through Run Command.
# Usage: deploy-to-server.sh <image-tag>
set -euo pipefail

image_tag="$1"

print_command_output() {
  aws ssm list-command-invocations --command-id "$1" --details \
    --query 'CommandInvocations[0].CommandPlugins[0].Output' --output text
}

aws ssm put-parameter --name "$IMAGE_TAG_PARAMETER" --value "$image_tag" --overwrite > /dev/null

command_id=$(aws ssm send-command \
  --document-name "$DEPLOY_DOCUMENT" \
  --targets "Key=tag:Name,Values=${INSTANCE_NAME}" \
  --comment "Deploy ${image_tag:0:7} from GitHub run ${GITHUB_RUN_ID}" \
  --query Command.CommandId --output text)
echo "Run Command ${command_id}: deploying ${image_tag}"

for _ in $(seq 1 36); do
  invocation_status=$(aws ssm list-command-invocations --command-id "$command_id" \
    --query 'CommandInvocations[0].Status' --output text)

  case "$invocation_status" in
    Success)
      print_command_output "$command_id"
      exit 0
      ;;
    None | Pending | InProgress | Delayed)
      sleep 5
      ;;
    *)
      print_command_output "$command_id"
      echo "::error::Deploy of ${image_tag} ended with status ${invocation_status}"
      exit 1
      ;;
  esac
done

echo "::error::Deploy of ${image_tag} did not finish within 3 minutes"
exit 1
