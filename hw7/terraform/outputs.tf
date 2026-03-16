output "alb_dns_name" {
  description = "Public DNS name for the ALB."
  value       = aws_lb.main.dns_name
}

output "sns_topic_arn" {
  description = "SNS topic ARN for order events."
  value       = aws_sns_topic.orders.arn
}

output "sqs_queue_url" {
  description = "SQS queue URL for order processing."
  value       = aws_sqs_queue.orders.id
}

output "ecr_repository_url" {
  description = "ECR repository URL for the ECS image."
  value       = aws_ecr_repository.app.repository_url
}
