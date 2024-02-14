output "rds_hostname" {
  description = "RDS instance hostname"
  value       = aws_db_instance.wordpress-db.address
  sensitive   = true
}

output "rds_db_name" {
  description = "RDS database name"
  value       = aws_db_instance.wordpress-db.db_name
  sensitive   = true
}

output "rds_port" {
  description = "RDS instance port"
  value       = aws_db_instance.wordpress-db.port
  sensitive   = true
}

output "rds_username" {
  description = "RDS instance root username"
  value       = aws_db_instance.wordpress-db.username
  sensitive   = true
}

output "rds_password" {
  description = "RDS instance root user password"
  value       = aws_db_instance.wordpress-db.password
  sensitive   = true
}

