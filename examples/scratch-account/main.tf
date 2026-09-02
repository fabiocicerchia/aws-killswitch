# One minimal resource of every kind aws-killswitch supports, in a throwaway
# account, so `aws-killswitch verify` has something to fire at.
#
# THIS COSTS MONEY WHILE IT EXISTS. A NAT gateway alone is roughly $32/month
# before data charges, and the RDS instances and the EKS nodegroup are not free
# either. `terraform destroy` when the run is done, and check the bill.
#
# WHAT IT IS FOR: the verification the tool has never had. The planner, the
# state machine and the ordering are unit-tested; the API contracts are not,
# because nothing had ever run with credentials that work. `verify` reads the
# account back after the fire and again after the restore, and a kind with no
# resource in the account is a kind it reports as UNEXERCISED — which is why
# this module exists rather than "point it at whatever you have lying around".
#
#   terraform init && terraform apply
#   aws-killswitch verify --config killswitch.json --yes
#   terraform destroy
#
# NOT APPLIED ANYWHERE. This was written from the provider's documented
# schemas; nobody has run `terraform apply` on it. Expect to fix an argument
# or two on the first run — and please fix them here rather than locally.

terraform {
  required_version = ">= 1.5"
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0" # VERSION-BUMP
    }
  }
}

provider "aws" {
  region = var.region
}

variable "region" {
  type    = string
  default = "eu-west-1"
}

variable "prefix" {
  type        = string
  default     = "ksverify"
  description = "Name prefix. Everything created here carries it, so a stray resource is identifiable after a failed destroy."
}

# The tag the killswitch policy scopes on. Every resource carries it, so a
# `verify` run cannot reach anything this module did not create — which is the
# only thing standing between a mistyped profile and somebody's production.
variable "scope_tag" {
  type    = map(string)
  default = { killswitch = "scratch" }
}

locals {
  tags = merge(var.scope_tag, { Name = var.prefix, ManagedBy = "aws-killswitch/examples/scratch-account" })
}

data "aws_availability_zones" "available" {
  state = "available"
}

# --- network -----------------------------------------------------------------

resource "aws_vpc" "main" {
  cidr_block           = "10.42.0.0/16"
  enable_dns_hostnames = true
  tags                 = local.tags
}

resource "aws_internet_gateway" "main" {
  vpc_id = aws_vpc.main.id
  tags   = local.tags
}

resource "aws_subnet" "public" {
  count                   = 2
  vpc_id                  = aws_vpc.main.id
  cidr_block              = cidrsubnet(aws_vpc.main.cidr_block, 8, count.index)
  availability_zone       = data.aws_availability_zones.available.names[count.index]
  map_public_ip_on_launch = true
  tags                    = local.tags
}

resource "aws_subnet" "private" {
  count             = 2
  vpc_id            = aws_vpc.main.id
  cidr_block        = cidrsubnet(aws_vpc.main.cidr_block, 8, count.index + 10)
  availability_zone = data.aws_availability_zones.available.names[count.index]
  tags              = local.tags
}

resource "aws_eip" "nat" {
  domain = "vpc"
  tags   = local.tags
}

# KindNATGateway. The one resource the killswitch DELETES rather than stops,
# because there is no way to stop one — which makes it the most important kind
# to have in the account: its restore path is a recreate, and a recreate is
# where a restore goes wrong.
resource "aws_nat_gateway" "main" {
  allocation_id = aws_eip.nat.id
  subnet_id     = aws_subnet.public[0].id
  tags          = local.tags
  depends_on    = [aws_internet_gateway.main]
}

resource "aws_security_group" "open" {
  name   = "${var.prefix}-sg"
  vpc_id = aws_vpc.main.id
  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }
  ingress {
    from_port   = 80
    to_port     = 80
    protocol    = "tcp"
    cidr_blocks = [aws_vpc.main.cidr_block]
  }
  tags = local.tags
}

# --- KindALBListener ----------------------------------------------------------

resource "aws_lb" "main" {
  name               = "${var.prefix}-alb"
  internal           = true
  load_balancer_type = "application"
  subnets            = aws_subnet.public[*].id
  security_groups    = [aws_security_group.open.id]
  tags               = local.tags
}

resource "aws_lb_target_group" "main" {
  name        = "${var.prefix}-tg"
  port        = 80
  protocol    = "HTTP"
  vpc_id      = aws_vpc.main.id
  target_type = "ip"
  tags        = local.tags
}

resource "aws_lb_listener" "main" {
  load_balancer_arn = aws_lb.main.arn
  port              = 80
  protocol          = "HTTP"
  default_action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.main.arn
  }
  tags = local.tags
}

# --- KindLambda ---------------------------------------------------------------

data "aws_iam_policy_document" "lambda_assume" {
  statement {
    actions = ["sts:AssumeRole"]
    principals {
      type        = "Service"
      identifiers = ["lambda.amazonaws.com"]
    }
  }
}

resource "aws_iam_role" "lambda" {
  name               = "${var.prefix}-lambda"
  assume_role_policy = data.aws_iam_policy_document.lambda_assume.json
  tags               = local.tags
}

data "archive_file" "noop" {
  type        = "zip"
  output_path = "${path.module}/.noop.zip"
  source {
    content  = "def handler(event, context):\n    return {}\n"
    filename = "index.py"
  }
}

resource "aws_lambda_function" "main" {
  function_name    = "${var.prefix}-fn"
  role             = aws_iam_role.lambda.arn
  handler          = "index.handler"
  runtime          = "python3.12" # VERSION-BUMP
  filename         = data.archive_file.noop.output_path
  source_code_hash = data.archive_file.noop.output_base64sha256
  # Set so the killswitch has something to restore. A function with NO reserved
  # concurrency and one throttled to zero look different in the API, and the
  # restore has to put back "unset" rather than "zero" — which is exactly the
  # kind of contract mismatch `verify` is looking for.
  reserved_concurrent_executions = 5
  tags                           = local.tags
}

# --- KindECSService -----------------------------------------------------------

resource "aws_ecs_cluster" "main" {
  name = "${var.prefix}-cluster"
  tags = local.tags
}

resource "aws_ecs_task_definition" "main" {
  family                   = "${var.prefix}-task"
  requires_compatibilities = ["FARGATE"]
  network_mode             = "awsvpc"
  cpu                      = 256
  memory                   = 512
  container_definitions = jsonencode([{
    name      = "app"
    image     = "public.ecr.aws/docker/library/busybox:latest" # VERSION-BUMP
    essential = true
    command   = ["sh", "-c", "sleep 86400"]
  }])
  tags = local.tags
}

resource "aws_ecs_service" "main" {
  name            = "${var.prefix}-svc"
  cluster         = aws_ecs_cluster.main.id
  task_definition = aws_ecs_task_definition.main.arn
  desired_count   = 2
  launch_type     = "FARGATE"
  network_configuration {
    subnets         = aws_subnet.private[*].id
    security_groups = [aws_security_group.open.id]
  }
  tags = local.tags
}

# --- KindASG and KindEC2Instance ---------------------------------------------

# arm64, and the instance types below match it. Graviton is what this account
# would be built on if it were real — greenlint (GL016) is a sibling tool in
# this same portfolio, and a scratch account that costs real money while it
# exists is the last place to ignore it. The AMI architecture and the instance
# family have to move together: an arm64 image will not boot on t3.
data "aws_ssm_parameter" "al2023" {
  name = "/aws/service/ami-amazon-linux-latest/al2023-ami-kernel-default-arm64"
}

resource "aws_launch_template" "main" {
  name_prefix   = "${var.prefix}-"
  image_id      = data.aws_ssm_parameter.al2023.value
  instance_type = "t4g.micro"
  tag_specifications {
    resource_type = "instance"
    tags          = local.tags
  }
}

resource "aws_autoscaling_group" "main" {
  name                = "${var.prefix}-asg"
  min_size            = 1
  desired_capacity    = 2
  max_size            = 3
  vpc_zone_identifier = aws_subnet.private[*].id
  launch_template {
    id      = aws_launch_template.main.id
    version = "$Latest"
  }
  dynamic "tag" {
    for_each = local.tags
    content {
      key                 = tag.key
      value               = tag.value
      propagate_at_launch = true
    }
  }
}

# A standalone instance, NOT in the ASG: the killswitch treats the two
# differently, and an account where every instance belongs to a group would
# never exercise the standalone path.
resource "aws_instance" "standalone" {
  ami                    = data.aws_ssm_parameter.al2023.value
  instance_type          = "t4g.micro"
  subnet_id              = aws_subnet.private[0].id
  vpc_security_group_ids = [aws_security_group.open.id]
  tags                   = local.tags
}

# --- KindRDSInstance and KindRDSCluster ---------------------------------------

resource "aws_db_subnet_group" "main" {
  name       = "${var.prefix}-subnets"
  subnet_ids = aws_subnet.private[*].id
  tags       = local.tags
}

resource "aws_db_instance" "main" {
  identifier             = "${var.prefix}-db"
  engine                 = "postgres"
  engine_version         = "16.4" # VERSION-BUMP
  instance_class         = "db.t4g.micro"
  allocated_storage      = 20
  username               = "ksverify"
  manage_master_user_password = true
  db_subnet_group_name   = aws_db_subnet_group.main.name
  vpc_security_group_ids = [aws_security_group.open.id]
  skip_final_snapshot    = true
  tags                   = local.tags
}

resource "aws_rds_cluster" "main" {
  cluster_identifier          = "${var.prefix}-cluster"
  engine                      = "aurora-postgresql"
  engine_mode                 = "provisioned"
  engine_version              = "16.4" # VERSION-BUMP
  master_username             = "ksverify"
  manage_master_user_password = true
  db_subnet_group_name        = aws_db_subnet_group.main.name
  vpc_security_group_ids      = [aws_security_group.open.id]
  skip_final_snapshot         = true
  serverlessv2_scaling_configuration {
    min_capacity = 0.5
    max_capacity = 1
  }
  tags = local.tags
}

resource "aws_rds_cluster_instance" "main" {
  identifier         = "${var.prefix}-cluster-1"
  cluster_identifier = aws_rds_cluster.main.id
  instance_class     = "db.serverless"
  engine             = aws_rds_cluster.main.engine
  tags               = local.tags
}

# --- KindEKSNodegroup ---------------------------------------------------------

data "aws_iam_policy_document" "eks_assume" {
  statement {
    actions = ["sts:AssumeRole"]
    principals {
      type        = "Service"
      identifiers = ["eks.amazonaws.com"]
    }
  }
}

data "aws_iam_policy_document" "node_assume" {
  statement {
    actions = ["sts:AssumeRole"]
    principals {
      type        = "Service"
      identifiers = ["ec2.amazonaws.com"]
    }
  }
}

resource "aws_iam_role" "eks" {
  name               = "${var.prefix}-eks"
  assume_role_policy = data.aws_iam_policy_document.eks_assume.json
  tags               = local.tags
}

resource "aws_iam_role_policy_attachment" "eks_cluster" {
  role       = aws_iam_role.eks.name
  policy_arn = "arn:aws:iam::aws:policy/AmazonEKSClusterPolicy"
}

resource "aws_iam_role" "node" {
  name               = "${var.prefix}-node"
  assume_role_policy = data.aws_iam_policy_document.node_assume.json
  tags               = local.tags
}

resource "aws_iam_role_policy_attachment" "node" {
  for_each = toset([
    "arn:aws:iam::aws:policy/AmazonEKSWorkerNodePolicy",
    "arn:aws:iam::aws:policy/AmazonEKS_CNI_Policy",
    "arn:aws:iam::aws:policy/AmazonEC2ContainerRegistryReadOnly",
  ])
  role       = aws_iam_role.node.name
  policy_arn = each.value
}

resource "aws_eks_cluster" "main" {
  name     = "${var.prefix}-eks"
  role_arn = aws_iam_role.eks.arn
  vpc_config {
    subnet_ids = concat(aws_subnet.public[*].id, aws_subnet.private[*].id)
  }
  tags       = local.tags
  depends_on = [aws_iam_role_policy_attachment.eks_cluster]
}

resource "aws_eks_node_group" "main" {
  cluster_name    = aws_eks_cluster.main.name
  node_group_name = "${var.prefix}-ng"
  node_role_arn   = aws_iam_role.node.arn
  subnet_ids      = aws_subnet.private[*].id
  # ami_type has to be set explicitly alongside an arm64 instance family: the
  # default is AL2023_x86_64_STANDARD, and a nodegroup that pairs it with t4g
  # gets nodes that never join.
  ami_type       = "AL2023_ARM_64_STANDARD"
  instance_types = ["t4g.small"]
  scaling_config {
    min_size     = 1
    desired_size = 2
    max_size     = 3
  }
  tags       = local.tags
  depends_on = [aws_iam_role_policy_attachment.node]
}

# --- KindAPIGatewayStage ------------------------------------------------------

resource "aws_api_gateway_rest_api" "main" {
  name = "${var.prefix}-api"
  body = jsonencode({
    openapi = "3.0.1"
    info    = { title = "${var.prefix}-api", version = "1" }
    paths = {
      "/" = {
        get = {
          x-amazon-apigateway-integration = {
            type                = "MOCK"
            requestTemplates    = { "application/json" = "{\"statusCode\": 200}" }
            passthroughBehavior = "when_no_match"
          }
          responses = { "200" = { description = "ok" } }
        }
      }
    }
  })
  tags = local.tags
}

resource "aws_api_gateway_deployment" "main" {
  rest_api_id = aws_api_gateway_rest_api.main.id
  triggers    = { redeploy = sha1(aws_api_gateway_rest_api.main.body) }
  lifecycle {
    create_before_destroy = true
  }
}

resource "aws_api_gateway_stage" "main" {
  rest_api_id   = aws_api_gateway_rest_api.main.id
  deployment_id = aws_api_gateway_deployment.main.id
  stage_name    = "verify"
  tags          = local.tags
}

# Throttling set explicitly, so the killswitch has a prior value to put back.
# A stage with no method settings and one throttled to zero are different in
# the API, and the restore has to know which it started from.
resource "aws_api_gateway_method_settings" "main" {
  rest_api_id = aws_api_gateway_rest_api.main.id
  stage_name  = aws_api_gateway_stage.main.stage_name
  method_path = "*/*"
  settings {
    throttling_rate_limit  = 100
    throttling_burst_limit = 50
  }
}

# --- KindCloudFront -----------------------------------------------------------

resource "aws_cloudfront_distribution" "main" {
  enabled = true
  comment = "${var.prefix} verification target"

  origin {
    domain_name = aws_lb.main.dns_name
    origin_id   = "alb"
    custom_origin_config {
      http_port              = 80
      https_port             = 443
      origin_protocol_policy = "http-only"
      origin_ssl_protocols   = ["TLSv1.2"]
    }
  }

  default_cache_behavior {
    target_origin_id       = "alb"
    viewer_protocol_policy = "allow-all"
    allowed_methods        = ["GET", "HEAD"]
    cached_methods         = ["GET", "HEAD"]
    forwarded_values {
      query_string = false
      cookies {
        forward = "none"
      }
    }
  }

  restrictions {
    geo_restriction {
      restriction_type = "none"
    }
  }

  viewer_certificate {
    cloudfront_default_certificate = true
  }

  tags = local.tags
}

output "next" {
  value = <<-EOT
    Scratch account seeded. Now:

      aws-killswitch plan   --config killswitch.json
      aws-killswitch verify --config killswitch.json --yes

    and when you are done, before the bill notices:

      terraform destroy
  EOT
}
