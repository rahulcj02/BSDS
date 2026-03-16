variable "project_name" {
  type        = string
  description = "Name prefix for all resources."
  default     = "cs6650-hw7"
}

variable "aws_region" {
  type        = string
  description = "AWS region to deploy into."
  default     = "us-west-2"
}

variable "container_image" {
  type        = string
  description = "Full image URI for the ECS task (ECR or other registry)."
}

variable "worker_count" {
  type        = number
  description = "Number of processor goroutines per ECS task."
  default     = 1
}

variable "use_existing_iam_roles" {
  type        = bool
  description = "Use pre-existing IAM roles instead of creating new ones."
  default     = false
}

variable "ecs_task_execution_role_arn" {
  type        = string
  description = "Existing ECS task execution role ARN (required if use_existing_iam_roles=true)."
  default     = ""
}

variable "ecs_task_role_arn" {
  type        = string
  description = "Existing ECS task role ARN (required if use_existing_iam_roles=true)."
  default     = ""
}

variable "lambda_role_arn" {
  type        = string
  description = "Existing Lambda execution role ARN (required if use_existing_iam_roles=true)."
  default     = ""
}
