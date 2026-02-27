variable "aws_region" {
  type    = string
  default = "us-west-2"
}

variable "service_name" {
  type    = string
  default = "hw6-search"
}

variable "ecr_repository_name" {
  type    = string
  default = "hw6-search"
}

variable "container_port" {
  type    = number
  default = 8080
}

variable "cpu" {
  type    = string
  default = "256"
}

variable "memory" {
  type    = string
  default = "512"
}

variable "ecs_desired_count" {
  type    = number
  default = 1
}

variable "autoscaling_min_capacity" {
  type    = number
  default = 1
}

variable "autoscaling_max_capacity" {
  type    = number
  default = 1
}

variable "autoscaling_target_cpu" {
  type    = number
  default = 70
}

variable "autoscaling_scale_in_cooldown" {
  type    = number
  default = 300
}

variable "autoscaling_scale_out_cooldown" {
  type    = number
  default = 300
}

variable "log_retention_days" {
  type    = number
  default = 7
}
