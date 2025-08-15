#!/bin/bash

# Script to generate architecture diagram as PNG
# This script uses Mermaid CLI or online service to convert the diagram

set -e

# Colors for output
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${BLUE}Generating Architecture Diagram...${NC}"

# Create docs directory if it doesn't exist
mkdir -p docs

# Extract the Mermaid diagram from the markdown file
echo -e "${BLUE}Extracting Mermaid diagram...${NC}"

# Create a temporary mermaid file
cat > docs/architecture.mmd << 'EOF'
graph TB
    %% Client Layer
    subgraph "Client Applications"
        Web[Web Apps]
        Mobile[Mobile Apps]
        API[API Clients]
        IoT[IoT Devices]
    end

    %% Load Balancer Layer
    subgraph "Load Balancer Layer"
        LB[Cloud Load Balancer<br/>NGINX/Traefik]
        CDN[CDN<br/>Cloudflare/AWS CloudFront]
    end

    %% API Gateway Layer
    subgraph "API Gateway Cluster"
        subgraph "Gateway Instances"
            GW1[API Gateway 1<br/>:8080]
            GW2[API Gateway 2<br/>:8080]
            GW3[API Gateway 3<br/>:8080]
            GW4[API Gateway N<br/>:8080]
        end
        
        subgraph "Gateway Components"
            Auth[JWT Auth<br/>& API Keys]
            RateLimit[Rate Limiting<br/>Distributed/Redis]
            Circuit[Circuit Breakers]
            Cache[Redis Cache<br/>Distributed]
            Proxy[Reverse Proxy<br/>Load Balancing]
            Events[Event Processor<br/>Kafka/RabbitMQ]
        end
        
        subgraph "Gateway Metrics"
            Prom[Prometheus<br/>Metrics :9090]
            Health[Health Checks<br/>& Monitoring]
        end
    end

    %% Microservices Layer
    subgraph "Microservices Layer"
        subgraph "Business Services"
            UserSvc[User Service<br/>:8080]
            OrderSvc[Order Service<br/>:8080]
            PaymentSvc[Payment Service<br/>:8080]
            NotificationSvc[Notification Service<br/>:8080]
            AnalyticsSvc[Analytics Service<br/>:8080]
        end
        
        subgraph "Service Discovery"
            Consul[Consul<br/>Service Registry]
            K8s[Kubernetes<br/>Service Discovery]
        end
    end

    %% Event Processing Layer
    subgraph "Event Processing Infrastructure"
        subgraph "Kafka Cluster"
            Kafka1[Kafka Broker 1<br/>:9092]
            Kafka2[Kafka Broker 2<br/>:9092]
            Kafka3[Kafka Broker 3<br/>:9092]
            ZK1[Zookeeper 1<br/>:2181]
            ZK2[Zookeeper 2<br/>:2181]
            ZK3[Zookeeper 3<br/>:2181]
        end
        
        subgraph "Event Topics"
            APIEvents[API Gateway Events]
            UserEvents[User Events]
            AuditLogs[Audit Logs]
            Metrics[Metrics Stream]
            Alerts[Alert Events]
        end
        
        subgraph "Event Consumers"
            Analytics[Analytics Consumer]
            Audit[Audit Consumer]
            Monitoring[Monitoring Consumer]
            Notification[Notification Consumer]
        end
    end

    %% Data Layer
    subgraph "Data Infrastructure"
        subgraph "Caching Layer"
            Redis1[Redis Node 1<br/>:6379]
            Redis2[Redis Node 2<br/>:6379]
            Redis3[Redis Node 3<br/>:6379]
            RedisCluster[Redis Cluster<br/>Rate Limiting & Cache]
        end
        
        subgraph "Database Layer"
            Postgres[PostgreSQL<br/>Primary :5432]
            PostgresReplica1[PostgreSQL<br/>Replica 1 :5432]
            PostgresReplica2[PostgreSQL<br/>Replica 2 :5432]
        end
        
        subgraph "Message Queue"
            RabbitMQ[RabbitMQ Cluster<br/>Alternative to Kafka]
        end
    end

    %% Monitoring Layer
    subgraph "Observability Stack"
        subgraph "Metrics & Monitoring"
            Prometheus[Prometheus<br/>:9090]
            Grafana[Grafana<br/>:3000]
            AlertManager[Alert Manager<br/>:9093]
        end
        
        subgraph "Tracing & Logging"
            Jaeger[Jaeger<br/>Distributed Tracing :16686]
            ELK[ELK Stack<br/>Logging & Analytics]
        end
        
        subgraph "Dashboards"
            GatewayDashboard[API Gateway<br/>Overview Dashboard]
            EventDashboard[Event Processing<br/>Dashboard]
            PerformanceDashboard[Performance<br/>Metrics Dashboard]
            SecurityDashboard[Security & Audit<br/>Dashboard]
        end
    end

    %% Security Layer
    subgraph "Security Infrastructure"
        subgraph "Authentication"
            OAuth[OAuth 2.0 Provider]
            JWT[JWT Token Service]
            APIKeys[API Key Management]
        end
        
        subgraph "Network Security"
            Firewall[Network Firewall]
            WAF[Web Application Firewall]
            VPN[VPN Access]
        end
        
        subgraph "Secrets Management"
            Vault[HashiCorp Vault]
            K8sSecrets[Kubernetes Secrets]
        end
    end

    %% CI/CD Layer
    subgraph "DevOps & CI/CD"
        subgraph "Version Control"
            Git[Git Repository<br/>GitHub/GitLab]
        end
        
        subgraph "CI/CD Pipeline"
            Build[Build Pipeline<br/>Docker Images]
            Test[Testing Pipeline<br/>Unit & Integration]
            Deploy[Deployment Pipeline<br/>Kubernetes]
        end
        
        subgraph "Infrastructure"
            Terraform[Terraform<br/>Infrastructure as Code]
            Helm[Helm Charts<br/>Kubernetes Deployment]
        end
    end

    %% Connection Lines
    %% Client to Load Balancer
    Web --> LB
    Mobile --> LB
    API --> LB
    IoT --> LB
    CDN --> LB

    %% Load Balancer to Gateway
    LB --> GW1
    LB --> GW2
    LB --> GW3
    LB --> GW4

    %% Gateway Internal Components
    GW1 --> Auth
    GW1 --> RateLimit
    GW1 --> Circuit
    GW1 --> Cache
    GW1 --> Proxy
    GW1 --> Events
    GW1 --> Prom
    GW1 --> Health

    GW2 --> Auth
    GW2 --> RateLimit
    GW2 --> Circuit
    GW2 --> Cache
    GW2 --> Proxy
    GW2 --> Events
    GW2 --> Prom
    GW2 --> Health

    GW3 --> Auth
    GW3 --> RateLimit
    GW3 --> Circuit
    GW3 --> Cache
    GW3 --> Proxy
    GW3 --> Events
    GW3 --> Prom
    GW3 --> Health

    GW4 --> Auth
    GW4 --> RateLimit
    GW4 --> Circuit
    GW4 --> Cache
    GW4 --> Proxy
    GW4 --> Events
    GW4 --> Prom
    GW4 --> Health

    %% Gateway to Microservices
    Proxy --> UserSvc
    Proxy --> OrderSvc
    Proxy --> PaymentSvc
    Proxy --> NotificationSvc
    Proxy --> AnalyticsSvc

    %% Service Discovery
    UserSvc --> Consul
    OrderSvc --> Consul
    PaymentSvc --> Consul
    NotificationSvc --> Consul
    AnalyticsSvc --> Consul

    %% Gateway to Event Processing
    Events --> Kafka1
    Events --> Kafka2
    Events --> Kafka3

    %% Kafka Cluster
    Kafka1 --> ZK1
    Kafka2 --> ZK2
    Kafka3 --> ZK3
    ZK1 --> ZK2
    ZK2 --> ZK3
    ZK3 --> ZK1

    %% Kafka Topics
    Kafka1 --> APIEvents
    Kafka2 --> UserEvents
    Kafka3 --> AuditLogs
    Kafka1 --> Metrics
    Kafka2 --> Alerts

    %% Event Consumers
    APIEvents --> Analytics
    UserEvents --> Analytics
    AuditLogs --> Audit
    Metrics --> Monitoring
    Alerts --> Notification

    %% Gateway to Data Layer
    RateLimit --> RedisCluster
    Cache --> RedisCluster
    RedisCluster --> Redis1
    RedisCluster --> Redis2
    RedisCluster --> Redis3

    %% Microservices to Database
    UserSvc --> Postgres
    OrderSvc --> Postgres
    PaymentSvc --> Postgres
    Postgres --> PostgresReplica1
    Postgres --> PostgresReplica2

    %% Alternative Message Queue
    Events --> RabbitMQ

    %% Monitoring Connections
    Prom --> Prometheus
    Health --> Prometheus
    Prometheus --> Grafana
    Prometheus --> AlertManager

    %% Tracing
    GW1 --> Jaeger
    GW2 --> Jaeger
    GW3 --> Jaeger
    GW4 --> Jaeger
    UserSvc --> Jaeger
    OrderSvc --> Jaeger
    PaymentSvc --> Jaeger

    %% Grafana Dashboards
    Grafana --> GatewayDashboard
    Grafana --> EventDashboard
    Grafana --> PerformanceDashboard
    Grafana --> SecurityDashboard

    %% Security
    Auth --> OAuth
    Auth --> JWT
    Auth --> APIKeys
    LB --> WAF
    WAF --> Firewall
    Vault --> K8sSecrets

    %% CI/CD
    Git --> Build
    Build --> Test
    Test --> Deploy
    Deploy --> Terraform
    Terraform --> Helm

    %% Styling
    classDef client fill:#e1f5fe,stroke:#01579b,stroke-width:2px
    classDef gateway fill:#f3e5f5,stroke:#4a148c,stroke-width:2px
    classDef service fill:#e8f5e8,stroke:#1b5e20,stroke-width:2px
    classDef event fill:#fff3e0,stroke:#e65100,stroke-width:2px
    classDef data fill:#fce4ec,stroke:#880e4f,stroke-width:2px
    classDef monitoring fill:#e0f2f1,stroke:#004d40,stroke-width:2px
    classDef security fill:#f1f8e9,stroke:#33691e,stroke-width:2px
    classDef devops fill:#fafafa,stroke:#424242,stroke-width:2px

    class Web,Mobile,API,IoT client
    class LB,CDN,GW1,GW2,GW3,GW4,Auth,RateLimit,Circuit,Cache,Proxy,Events,Prom,Health gateway
    class UserSvc,OrderSvc,PaymentSvc,NotificationSvc,AnalyticsSvc,Consul,K8s service
    class Kafka1,Kafka2,Kafka3,ZK1,ZK2,ZK3,APIEvents,UserEvents,AuditLogs,Metrics,Alerts,Analytics,Audit,Monitoring,Notification,RabbitMQ event
    class Redis1,Redis2,Redis3,RedisCluster,Postgres,PostgresReplica1,PostgresReplica2 data
    class Prometheus,Grafana,AlertManager,Jaeger,ELK,GatewayDashboard,EventDashboard,PerformanceDashboard,SecurityDashboard monitoring
    class OAuth,JWT,APIKeys,Firewall,WAF,VPN,Vault,K8sSecrets security
    class Git,Build,Test,Deploy,Terraform,Helm devops
EOF

echo -e "${GREEN}Mermaid diagram extracted to docs/architecture.mmd${NC}"

# Check if mmdc (Mermaid CLI) is available
if command -v mmdc &> /dev/null; then
    echo -e "${BLUE}Using Mermaid CLI to generate PNG...${NC}"
    mmdc -i docs/architecture.mmd -o docs/architecture.png -b transparent
    echo -e "${GREEN}Architecture diagram generated: docs/architecture.png${NC}"
elif command -v docker &> /dev/null; then
    echo -e "${BLUE}Using Docker to generate PNG...${NC}"
    docker run --rm -v "$(pwd)/docs:/data" minlag/mermaid-cli:latest -i /data/architecture.mmd -o /data/architecture.png -b transparent
    echo -e "${GREEN}Architecture diagram generated: docs/architecture.png${NC}"
else
    echo -e "${YELLOW}Mermaid CLI not found. Please install it or use the online service.${NC}"
    echo -e "${BLUE}You can generate the PNG manually by:${NC}"
    echo -e "1. Go to https://mermaid.live"
    echo -e "2. Copy the content from docs/architecture.mmd"
    echo -e "3. Paste it into the editor"
    echo -e "4. Export as PNG"
    echo -e "5. Save as docs/architecture.png"
fi

# Clean up temporary file
rm -f docs/architecture.mmd

echo -e "${GREEN}Architecture diagram generation completed!${NC}"
