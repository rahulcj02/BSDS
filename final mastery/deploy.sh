#!/bin/bash
# ============================================================================
# Album Store - AWS Deployment Script
# Run this on your LOCAL machine with AWS CLI configured
# ============================================================================

set -e

# ── CONFIGURATION (EDIT THESE) ──────────────────────────────────────────────
AWS_REGION="us-west-2"
KEY_PAIR_NAME="album-store-key"        # Your EC2 key pair name
S3_BUCKET="album-store-photos-$(date +%s)"  # Unique bucket name
DB_PASSWORD="YourStrongPassword123!"   # Change this!
INSTANCE_TYPE="t3.medium"             # Good balance of CPU/memory

echo "============================================"
echo "  Album Store AWS Deployment"
echo "============================================"

# ── STEP 1: Create S3 Bucket ────────────────────────────────────────────────
echo ""
echo "[1/6] Creating S3 bucket: $S3_BUCKET"
aws s3 mb s3://$S3_BUCKET --region $AWS_REGION

# Make objects publicly readable (needed so ChaosArena can fetch photos)
aws s3api put-public-access-block \
  --bucket $S3_BUCKET \
  --public-access-block-configuration \
  "BlockPublicAcls=false,IgnorePublicAcls=false,BlockPublicPolicy=false,RestrictPublicBuckets=false"

aws s3api put-bucket-policy --bucket $S3_BUCKET --policy "{
  \"Version\": \"2012-10-17\",
  \"Statement\": [{
    \"Sid\": \"PublicReadGetObject\",
    \"Effect\": \"Allow\",
    \"Principal\": \"*\",
    \"Action\": \"s3:GetObject\",
    \"Resource\": \"arn:aws:s3:::$S3_BUCKET/*\"
  }]
}"
echo "   ✓ S3 bucket created and public read enabled"

# ── STEP 2: Create Security Group ───────────────────────────────────────────
echo ""
echo "[2/6] Creating security group"
VPC_ID=$(aws ec2 describe-vpcs --filters "Name=isDefault,Values=true" --query "Vpcs[0].VpcId" --output text --region $AWS_REGION)
SG_ID=$(aws ec2 create-security-group \
  --group-name album-store-sg \
  --description "Album Store security group" \
  --vpc-id $VPC_ID \
  --region $AWS_REGION \
  --query "GroupId" --output text)

# Allow inbound HTTP on 8080
aws ec2 authorize-security-group-ingress --group-id $SG_ID --protocol tcp --port 8080 --cidr 0.0.0.0/0 --region $AWS_REGION
# Allow SSH
aws ec2 authorize-security-group-ingress --group-id $SG_ID --protocol tcp --port 22 --cidr 0.0.0.0/0 --region $AWS_REGION
echo "   ✓ Security group created: $SG_ID"

# ── STEP 3: Create RDS PostgreSQL ───────────────────────────────────────────
echo ""
echo "[3/6] Creating RDS PostgreSQL instance (this takes 5-10 minutes)..."

# Create DB security group
DB_SG_ID=$(aws ec2 create-security-group \
  --group-name album-store-db-sg \
  --description "Album Store DB security group" \
  --vpc-id $VPC_ID \
  --region $AWS_REGION \
  --query "GroupId" --output text)

aws ec2 authorize-security-group-ingress --group-id $DB_SG_ID --protocol tcp --port 5432 --source-group $SG_ID --region $AWS_REGION

aws rds create-db-instance \
  --db-instance-identifier album-store-db \
  --db-instance-class db.t3.micro \
  --engine postgres \
  --engine-version 16.4 \
  --master-username albumuser \
  --master-user-password "$DB_PASSWORD" \
  --allocated-storage 20 \
  --vpc-security-group-ids $DB_SG_ID \
  --db-name albumstore \
  --publicly-accessible \
  --no-multi-az \
  --region $AWS_REGION \
  --storage-type gp3 \
  > /dev/null

echo "   Waiting for RDS to be available..."
aws rds wait db-instance-available --db-instance-identifier album-store-db --region $AWS_REGION
DB_HOST=$(aws rds describe-db-instances --db-instance-identifier album-store-db --query "DBInstances[0].Endpoint.Address" --output text --region $AWS_REGION)
echo "   ✓ RDS ready at: $DB_HOST"

# ── STEP 4: Create IAM Role for EC2 ─────────────────────────────────────────
echo ""
echo "[4/6] Creating IAM role for EC2"
aws iam create-role \
  --role-name album-store-ec2-role \
  --assume-role-policy-document '{
    "Version": "2012-10-17",
    "Statement": [{
      "Effect": "Allow",
      "Principal": {"Service": "ec2.amazonaws.com"},
      "Action": "sts:AssumeRole"
    }]
  }' > /dev/null 2>&1 || true

aws iam put-role-policy \
  --role-name album-store-ec2-role \
  --policy-name s3-access \
  --policy-document "{
    \"Version\": \"2012-10-17\",
    \"Statement\": [{
      \"Effect\": \"Allow\",
      \"Action\": [\"s3:PutObject\", \"s3:GetObject\", \"s3:DeleteObject\"],
      \"Resource\": \"arn:aws:s3:::$S3_BUCKET/*\"
    }]
  }"

aws iam create-instance-profile --instance-profile-name album-store-profile > /dev/null 2>&1 || true
aws iam add-role-to-instance-profile --instance-profile-name album-store-profile --role-name album-store-ec2-role > /dev/null 2>&1 || true
sleep 10  # Wait for IAM propagation
echo "   ✓ IAM role created"

# ── STEP 5: Launch EC2 Instance ──────────────────────────────────────────────
echo ""
echo "[5/6] Launching EC2 instance"

# Get latest Amazon Linux 2023 AMI
AMI_ID=$(aws ec2 describe-images \
  --owners amazon \
  --filters "Name=name,Values=al2023-ami-2023*-x86_64" "Name=state,Values=available" \
  --query "sort_by(Images, &CreationDate)[-1].ImageId" \
  --output text --region $AWS_REGION)

# User data script to set up the instance
USER_DATA=$(cat <<EOF
#!/bin/bash
yum update -y
yum install -y docker git
systemctl start docker
systemctl enable docker

# Install docker-compose
curl -L "https://github.com/docker/compose/releases/latest/download/docker-compose-\$(uname -s)-\$(uname -m)" -o /usr/local/bin/docker-compose
chmod +x /usr/local/bin/docker-compose

# Install Go
wget -q https://go.dev/dl/go1.21.13.linux-amd64.tar.gz
tar -C /usr/local -xzf go1.21.13.linux-amd64.tar.gz
export PATH=\$PATH:/usr/local/bin:/usr/local/go/bin

# Clone or create app directory
mkdir -p /home/ec2-user/album-store
chown ec2-user:ec2-user /home/ec2-user/album-store
EOF
)

INSTANCE_ID=$(aws ec2 run-instances \
  --image-id $AMI_ID \
  --instance-type $INSTANCE_TYPE \
  --key-name $KEY_PAIR_NAME \
  --security-group-ids $SG_ID \
  --iam-instance-profile Name=album-store-profile \
  --user-data "$USER_DATA" \
  --region $AWS_REGION \
  --query "Instances[0].InstanceId" --output text)

echo "   Waiting for instance to start..."
aws ec2 wait instance-running --instance-ids $INSTANCE_ID --region $AWS_REGION
PUBLIC_IP=$(aws ec2 describe-instances --instance-ids $INSTANCE_ID --query "Reservations[0].Instances[0].PublicIpAddress" --output text --region $AWS_REGION)
echo "   ✓ EC2 instance running: $PUBLIC_IP"

# ── STEP 6: Print Summary ───────────────────────────────────────────────────
echo ""
echo "============================================"
echo "  DEPLOYMENT SUMMARY"
echo "============================================"
echo ""
echo "EC2 Public IP:   $PUBLIC_IP"
echo "EC2 Instance ID: $INSTANCE_ID"
echo "RDS Endpoint:    $DB_HOST"
echo "S3 Bucket:       $S3_BUCKET"
echo "Security Group:  $SG_ID"
echo ""
echo "── NEXT STEPS ──"
echo ""
echo "1. Wait 2-3 minutes for EC2 user-data to finish"
echo ""
echo "2. SSH into the instance:"
echo "   ssh -i $KEY_PAIR_NAME.pem ec2-user@$PUBLIC_IP"
echo ""
echo "3. Copy your code to the instance:"
echo "   scp -i $KEY_PAIR_NAME.pem -r ./* ec2-user@$PUBLIC_IP:~/album-store/"
echo ""
echo "4. On the instance, build and run:"
echo "   cd ~/album-store"
echo "   export DATABASE_URL='postgres://albumuser:$DB_PASSWORD@$DB_HOST:5432/albumstore?sslmode=disable'"
echo "   export S3_BUCKET='$S3_BUCKET'"
echo "   export AWS_REGION='$AWS_REGION'"
echo "   export BASE_URL='http://$PUBLIC_IP:8080'"
echo "   export PORT='8080'"
echo "   /usr/local/go/bin/go mod tidy"
echo "   /usr/local/go/bin/go build -o album-store ."
echo "   nohup ./album-store > app.log 2>&1 &"
echo ""
echo "5. Test:"
echo "   curl http://$PUBLIC_IP:8080/health"
echo ""
echo "6. Submit to ChaosArena:"
echo "   curl -X POST http://chaosarena-alb-938452724.us-west-2.elb.amazonaws.com/submit \\"
echo "     -H 'Content-Type: application/json' \\"
echo "     -d '{\"email\":\"YOUR_EMAIL@northeastern.edu\",\"nickname\":\"YOUR_NICKNAME\",\"base_url\":\"http://$PUBLIC_IP:8080\",\"contract\":\"v1-album-store\"}'"
echo ""
echo "Save these values — you'll need them!"
echo "S3_BUCKET=$S3_BUCKET" > .env.deployed
echo "DB_HOST=$DB_HOST" >> .env.deployed
echo "PUBLIC_IP=$PUBLIC_IP" >> .env.deployed
echo "INSTANCE_ID=$INSTANCE_ID" >> .env.deployed
