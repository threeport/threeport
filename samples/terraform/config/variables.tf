variable "region" {
    description = "The region to deploy the RDS instance to"
    type = string
}

variable "vpc_id" {
    description = "The VPC to deploy the RDS instance to"
    type = string
}

variable "subnet_ids" {
    description = "The collection of subnets the RDS instance can be provisioned in"
        type = list
}

variable "app_security_group" {
    description = "The security group used by the application that incoming collections will be allowed from"
    type = string
}

variable "db_port" {
    description = "The port on which the database will accept connections"
    type = number
}

variable "db_password" {
  description = "RDS root user password"
  type        = string
  sensitive   = true
}
