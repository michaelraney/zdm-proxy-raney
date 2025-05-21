# Zero Downtime Migration (ZDM) Proxy on Amazon Web Services (AWS)

## Overview

This directory contains best practices and configuration guidelines for deploying the ZDM Proxy on AWS. 


## Amazon CloudWatch integration
Metrics can be sent directly to [Amazon CloudWatch](https://aws.amazon.com/cloudwatch/). This requires [Amazon IAM credentials](https://docs.aws.amazon.com/cli/latest/userguide/getting-started-quickstart.html) to be configured (via environment variables, IAM roles, etc.). Metrics for CloudWatch are gathered and reported on a configured interval on a background routine.

- Metrics are collected in memory and reported in batches
- Default batch size is 20 metrics per API call (CloudWatch quota per PutMetricData call)
- Default reporting interval is 1 minute (configurable)
- Latency Metrics are automatically aggregated to reduce API calls (Avg, Sum, Min, Max, SampleCount)

## Configuration
To enable the metrics for CloudWatch, you need to turn on metrics_enabled and define the metric type as amazoncloudwatch. All other parameters are optional. You will need to set up credentials for AWS SDK and provide a service role that can perform the ```cloudwatch:PutMetricData``` action. 


### ZDMProxy yaml configuration parameters

```yaml
metrics_enabled: true #required to enable metrics
metrics_type: amazoncloudwatch #required to enable Amazon CloudWatch
metrics_prefix: zdm # optional, defaults to zdm
amazon_cloudwatch_region: us-west-2  # optional, defaults to environment
amazon_cloudwatch_namespace: ZDMProxy  # optional, defaults to ZDMProxy
amazon_cloudwatch_report_interval_minutes: 1
```

### ZDM Environment Variable Configuration:

```bash
export ZDM_METRICS_ENABLED=true
export ZDM_METRICS_TYPE=amazoncloudwatch
export ZDM_METRICS_PREFIX=zdm
export ZDM_AMAZON_CLOUDWATCH_NAMESPACE=ZDMProxy
export ZDM_AMAZON_CLOUDWATCH_REGION=us-west-2
export ZDM_AMAZON_CLOUDWATCH_REPORT_INTERVAL_MINUTES=1
```

### AWS Identity Access Management (IAM) configuration to ZDMProxy metrics
The following permissions will allow the proxy to publish metric data to CloudWatch namespace 'ZDMProxy'. You will notice the above example defined ```ZDM_AMAZON_CLOUDWATCH_NAMESPACE``` environment variable or ZDM config parameter ```amazon_cloudwatch_namespace``` as ZDMProxy. Using the condition restricts the proxy to the defined namespace. 

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": "cloudwatch:PutMetricData",
      "Resource": "*",
      "Condition": {
        "StringEquals": {
          "cloudwatch:Namespace": "ZDMProxy"
        }
      }
    }
  ]
}
```

### Visualizing ZDMProxy CloudWatch metrics

Use the AWS CloudFormation template to set up a CloudWatch Dashboard to view ZDMProxy metrics. The dashboard will be created in your AWS account and will be accessible through the CloudWatch console. The template creates a comprehensive dashboard with the following metrics:
- Connection counts
- Request latencies 
- Failed requests and operations
- Stream ID usage
- Concurrent operations
- Async request latencies 

#### Deploying the CloudWatch Dashboard

You can deploy the template through the [AWS CloudFormation Console](https://us-east-1.console.aws.amazon.com/cloudformation/home) or from the AWS CLI. Ensure you have AWS CLI configured with appropriate credentials. 


You can customize the dashboard parameters during deployment:
- `CloudwatchDashBoardNameParameter`: Name of your CloudWatch dashboard
- `ProxyCloudwatchNamespace`: The namespace where your metrics are published
- `ProxyCloudwatchMetricPrefix`: The prefix used for your metrics

Example of how to deploy the CloudWatch dashboard using CloudFormation:

```bash
aws cloudformation deploy \
  --template-file zdm-proxy-cloudformation-cloudwatch-dashboard.yaml \
  --stack-name zdm-proxy-dashboard \
  --parameter-overrides \
    CloudwatchDashBoardNameParameter=ZDMProxyDashboard \
    ProxyCloudwatchNamespace=ZDMProxy \
    ProxyCloudwatchMetricPrefix=zdm
```






