variable "service_name" {
  type = string
}

variable "image" {
  type = string
}

variable "container_port" {
  type = number
}

variable "subnet_ids" {
  type = list(string)
}

variable "service_security_group_id" {
  type = string
}

variable "alb_security_group_id" {
  type = string
}

variable "vpc_id" {
  type = string
}

variable "execution_role_arn" {
  type = string
}

variable "task_role_arn" {
  type = string
}

variable "log_group_name" {
  type = string
}

variable "region" {
  type = string
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
