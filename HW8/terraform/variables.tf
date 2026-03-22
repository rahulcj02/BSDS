variable "aws_region" {
  type    = string
  default = "us-west-2"
}

variable "project_name" {
  type    = string
  default = "cs6650-hw8"
}

variable "container_image" {
  type        = string
  description = "Full image URI for the ECS task (ECR or other registry)."
}

variable "db_name" {
  type    = string
  default = "hw8"
}

variable "db_user" {
  type    = string
  default = "hw8user"
}

variable "ddb_table" {
  type    = string
  default = "shopping_carts"
}

variable "db_backend" {
  type    = string
  default = "mysql"
}

variable "use_existing_iam_roles" {
  type    = bool
  default = false
}

variable "ecs_task_execution_role_arn" {
  type    = string
  default = ""
}

variable "ecs_task_role_arn" {
  type    = string
  default = ""
}
