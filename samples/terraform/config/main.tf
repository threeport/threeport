terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 4.16"
    }
  }

  required_version = ">= 1.2.0"
}

provider "aws" {
  region  = var.region
}

resource "aws_db_subnet_group" "wordpress-db-subnet-group" {
  name       = "wordpress-db-subnet-group"
  subnet_ids = var.subnet_ids

  tags = {
    Name = "wordpress-db-subnet-group"
  }
}

resource "aws_security_group" "wordpress-db-sg" {
  name        = "wordpress-db-sg"
  description = "Allow database connection to database from wordpress"
  vpc_id      = var.vpc_id

  tags = {
    Name = "wordpress-db-sg"
  }
}

resource "aws_security_group_rule" "wordpress-db-ingress" {
  type                     = "ingress"
  security_group_id        = aws_security_group.wordpress-db-sg.id
  source_security_group_id = var.app_security_group
  from_port                = var.db_port
  protocol              = "tcp"
  to_port                  = var.db_port
}

resource "aws_db_instance" "wordpress-db" {
  identifier             = "wordpress-db"
  db_name                = "wordpress"
  instance_class         = "db.t3.micro"
  allocated_storage      = 20
  engine                 = "mariadb"
  engine_version         = "10.11"
  port                   = var.db_port
  username               = "wordpress_user"
  password               = var.db_password
  db_subnet_group_name   = aws_db_subnet_group.wordpress-db-subnet-group.name
  vpc_security_group_ids = [aws_security_group.wordpress-db-sg.id]
  publicly_accessible    = false
  skip_final_snapshot    = true
}
