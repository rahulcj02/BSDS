output "alb_dns_name" {
  value = aws_lb.main.dns_name
}

output "ecr_repository_url" {
  value = aws_ecr_repository.app.repository_url
}

output "rds_endpoint" {
  value = aws_db_instance.mysql.address
}

output "rds_username" {
  value = var.db_user
}

output "rds_password" {
  value     = random_password.db_password.result
  sensitive = true
}

output "dynamodb_table" {
  value = aws_dynamodb_table.carts.name
}

output "ecs_cluster_name" {
  value = aws_ecs_cluster.main.name
}

output "ecs_service_name" {
  value = aws_ecs_service.api.name
}
