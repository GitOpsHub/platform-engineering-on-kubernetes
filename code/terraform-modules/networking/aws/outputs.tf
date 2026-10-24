output "vpc_id" {
  description = "ID of the VPC."
  value       = aws_vpc.this.id
}

output "vpc_cidr_block" {
  description = "CIDR block of the VPC."
  value       = aws_vpc.this.cidr_block
}

output "private_subnet_ids" {
  description = "IDs of the private subnets (use for EKS nodes and the control-plane ENIs)."
  value       = aws_subnet.private[*].id
}

output "public_subnet_ids" {
  description = "IDs of the public subnets (internet-facing load balancers)."
  value       = aws_subnet.public[*].id
}

output "availability_zones" {
  description = "Availability zones used."
  value       = local.azs
}

output "nat_gateway_public_ips" {
  description = "Public egress IPs of the NAT gateways (useful for allow-listing)."
  value       = aws_eip.nat[*].public_ip
}
