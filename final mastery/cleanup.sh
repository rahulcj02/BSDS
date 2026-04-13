#!/bin/bash
# Tear down all AWS resources to avoid charges
# Run from your LOCAL machine

AWS_REGION="us-west-2"

echo "=== Tearing down Album Store AWS resources ==="

# Load saved values if available
if [ -f .env.deployed ]; then
    source .env.deployed
fi

# Terminate EC2
if [ -n "$INSTANCE_ID" ]; then
    echo "Terminating EC2 instance $INSTANCE_ID..."
    aws ec2 terminate-instances --instance-ids $INSTANCE_ID --region $AWS_REGION
    aws ec2 wait instance-terminated --instance-ids $INSTANCE_ID --region $AWS_REGION
    echo "  ✓ EC2 terminated"
fi

# Delete RDS
echo "Deleting RDS instance..."
aws rds delete-db-instance --db-instance-identifier album-store-db --skip-final-snapshot --region $AWS_REGION 2>/dev/null || true
echo "  ✓ RDS deletion initiated (takes a few minutes)"

# Empty and delete S3 bucket
if [ -n "$S3_BUCKET" ]; then
    echo "Deleting S3 bucket $S3_BUCKET..."
    aws s3 rm s3://$S3_BUCKET --recursive --region $AWS_REGION 2>/dev/null || true
    aws s3 rb s3://$S3_BUCKET --region $AWS_REGION 2>/dev/null || true
    echo "  ✓ S3 bucket deleted"
fi

# Delete security groups
echo "Deleting security groups..."
aws ec2 delete-security-group --group-name album-store-sg --region $AWS_REGION 2>/dev/null || true
aws ec2 delete-security-group --group-name album-store-db-sg --region $AWS_REGION 2>/dev/null || true
echo "  ✓ Security groups deleted"

# Delete IAM
echo "Cleaning up IAM..."
aws iam remove-role-from-instance-profile --instance-profile-name album-store-profile --role-name album-store-ec2-role 2>/dev/null || true
aws iam delete-instance-profile --instance-profile-name album-store-profile 2>/dev/null || true
aws iam delete-role-policy --role-name album-store-ec2-role --policy-name s3-access 2>/dev/null || true
aws iam delete-role --role-name album-store-ec2-role 2>/dev/null || true
echo "  ✓ IAM cleaned up"

echo ""
echo "=== Cleanup complete ==="
