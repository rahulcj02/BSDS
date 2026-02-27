output "ecs_cluster_name" {
  value = module.ecs.cluster_name
}

output "ecs_service_name" {
  value = module.ecs.service_name
}

output "alb_dns_name" {
  value = module.ecs.alb_dns_name
}

output "target_group_arn" {
  value = module.ecs.target_group_arn
}

output "ecr_repository_url" {
  value = module.ecr.repository_url
}
