module "network" {
  source         = "./modules/network"
  service_name   = var.service_name
  container_port = var.container_port
}

module "ecr" {
  source          = "./modules/ecr"
  repository_name = var.ecr_repository_name
}

module "logging" {
  source            = "./modules/logging"
  service_name      = var.service_name
  retention_in_days = var.log_retention_days
}

data "aws_iam_role" "lab_role" {
  name = "LabRole"
}

locals {
  src_files           = fileset("${path.module}/../src", "**")
  src_hash            = sha1(join("", [for f in local.src_files : filesha1("${path.module}/../src/${f}")]))
  image_build_version = "linux-amd64-v1"
}

resource "terraform_data" "build_and_push_image" {
  triggers_replace = [
    local.src_hash,
    local.image_build_version,
    module.ecr.repository_url,
  ]

  provisioner "local-exec" {
    command     = <<-EOT
      aws ecr get-login-password --region ${var.aws_region} | docker login --username AWS --password-stdin ${module.ecr.repository_url}
      docker build --platform linux/amd64 -t ${module.ecr.repository_url}:latest ../src
      docker push ${module.ecr.repository_url}:latest
    EOT
    working_dir = path.module
  }
}

module "ecs" {
  source                         = "./modules/ecs"
  service_name                   = var.service_name
  image                          = "${module.ecr.repository_url}:latest"
  container_port                 = var.container_port
  subnet_ids                     = module.network.subnet_ids
  service_security_group_id      = module.network.service_security_group_id
  alb_security_group_id          = module.network.alb_security_group_id
  vpc_id                         = module.network.vpc_id
  execution_role_arn             = data.aws_iam_role.lab_role.arn
  task_role_arn                  = data.aws_iam_role.lab_role.arn
  log_group_name                 = module.logging.log_group_name
  region                         = var.aws_region
  cpu                            = var.cpu
  memory                         = var.memory
  ecs_desired_count              = var.ecs_desired_count
  autoscaling_min_capacity       = var.autoscaling_min_capacity
  autoscaling_max_capacity       = var.autoscaling_max_capacity
  autoscaling_target_cpu         = var.autoscaling_target_cpu
  autoscaling_scale_in_cooldown  = var.autoscaling_scale_in_cooldown
  autoscaling_scale_out_cooldown = var.autoscaling_scale_out_cooldown
  depends_on                     = [terraform_data.build_and_push_image]
}
