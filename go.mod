module github.com/fabiocicerchia/aws-killswitch

go 1.24

toolchain go1.26.5

require (
	github.com/aws/aws-sdk-go-v2 v1.43.6
	github.com/aws/aws-sdk-go-v2/config v1.32.36
	github.com/aws/aws-sdk-go-v2/service/autoscaling v1.71.1
	github.com/aws/aws-sdk-go-v2/service/costexplorer v1.67.5
	github.com/aws/aws-sdk-go-v2/service/ec2 v1.318.1
	github.com/aws/aws-sdk-go-v2/service/ecs v1.90.0
	github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2 v1.58.4
	github.com/aws/aws-sdk-go-v2/service/lambda v1.101.1
	github.com/aws/aws-sdk-go-v2/service/rds v1.124.3
	github.com/aws/aws-sdk-go-v2/service/s3 v1.107.0
	github.com/aws/aws-sdk-go-v2/service/sts v1.45.5
)

require (
	github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream v1.7.16 // indirect
	github.com/aws/aws-sdk-go-v2/credentials v1.19.35 // indirect
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.18.36 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.4.37 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.7.37 // indirect
	github.com/aws/aws-sdk-go-v2/internal/v4a v1.4.37 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.13.17 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/checksum v1.9.28 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.13.37 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/s3shared v1.19.36 // indirect
	github.com/aws/aws-sdk-go-v2/service/signin v1.5.5 // indirect
	github.com/aws/aws-sdk-go-v2/service/sso v1.33.5 // indirect
	github.com/aws/aws-sdk-go-v2/service/ssooidc v1.38.5 // indirect
	github.com/aws/smithy-go v1.27.8 // indirect
)
