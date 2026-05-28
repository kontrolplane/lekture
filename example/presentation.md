---
author: levi van noort
date: YYYY-MM-dd
paging: "%d / %d"
---

# AWS Networking Deep Dive

### VPC, Route 53, CloudFront & ELB

---

## Agenda

We will cover the four pillars of AWS networking:

1. **VPC** — your private network in the cloud
2. **Security** — security groups, NACLs, and traffic filtering
3. **Load Balancing** — distributing traffic with ALB, NLB, and GLB
4. **CloudFront** — edge caching and content delivery
5. **Route 53** — DNS, health checks, and routing policies
6. **Tying it together** — a production-grade reference architecture

Each section includes real CIDR examples, gotchas we have hit in production, and cost breakdowns where they matter.

---

## What is a VPC?

A **Virtual Private Cloud** is your own logically isolated network inside AWS. Nothing gets in or out unless you explicitly allow it.

- Scoped to a single **region**, but spans all its availability zones
- You pick the IP range (**CIDR block**), carve it into **subnets**, and wire up **route tables**
- Every new account gets a default VPC — never use it for real workloads
- Supports dual-stack: **IPv4**, **IPv6**, or both simultaneously

A VPC itself is free. You pay for the things you attach to it: NAT Gateways, VPN tunnels, Transit Gateway attachments, and cross-AZ data transfer.

> We migrated off the default VPC in 2023 after an intern accidentally exposed
> an RDS instance. Custom VPCs with no IGW on private subnets would have
> prevented that entirely.

---

## VPC Building Blocks

| Component            | What It Does                                      |
|----------------------|---------------------------------------------------|
| **CIDR Block**       | Defines the IP range — e.g. `10.0.0.0/16`         |
| **Subnet**           | A slice of that range, pinned to one AZ            |
| **Route Table**      | Decides where packets go next                      |
| **Internet Gateway** | Two-way door to the public internet                |
| **NAT Gateway**      | One-way door: private subnet can reach out, nobody reaches in |
| **Security Group**   | Stateful firewall on each network interface         |
| **NACL**             | Stateless firewall on each subnet                   |
| **VPC Endpoint**     | Private path to AWS services — skips the internet   |
| **Peering**          | Direct link between two VPCs (no transitive routing)|
| **Transit Gateway**  | Hub-and-spoke router for dozens of VPCs + on-prem   |

The expensive bits: NAT Gateway runs **$0.045/hr** plus **$0.045/GB** processed. A busy private subnet can rack up hundreds of dollars a month just on NAT. VPC Endpoints for S3 and DynamoDB are free and eliminate that cost for those services.

---

## Subnets in Practice

**Public subnet** — has a route table entry pointing `0.0.0.0/0` to an Internet Gateway.

- ALBs, bastion hosts, NAT Gateways live here
- EC2 instances need a public IP or Elastic IP to be reachable
- Set `MapPublicIpOnLaunch` on the subnet so you don't forget per-instance

**Private subnet** — default route goes to a NAT Gateway (or nowhere).

- Application servers, databases, Lambda, ECS tasks
- Can still reach the internet outbound through NAT for patches, API calls
- Use **Gateway Endpoints** for S3/DynamoDB — they're free and faster

In production we run **three of each** across `eu-west-1a`, `1b`, and `1c`. That gives us AZ-level fault tolerance for every tier. The ALB requires at least two AZs anyway, so three is the natural minimum for serious workloads.

---

## CIDR Planning

This is the part that bites you later if you rush it.

| Block           | Size    | What We Use It For                  |
|-----------------|---------|-------------------------------------|
| `10.0.0.0/16`   | 65,536  | The VPC itself                      |
| `10.0.0.0/20`   | 4,096   | Public subnets (one per AZ)         |
| `10.0.48.0/20`  | 4,096   | Private subnets (one per AZ)        |
| `10.0.96.0/20`  | 4,096   | Data tier subnets (RDS, ElastiCache)|
| `10.0.144.0/20` | 4,096   | Spare — future growth               |

AWS steals **5 IPs** from every subnet: `.0` (network), `.1` (router), `.2` (DNS), `.3` (reserved), and the last address (broadcast). On a `/28` that is 5 out of 16 — nearly a third.

Things to remember:

- VPC CIDR ranges go from `/16` down to `/28` — you cannot create anything larger
- **You cannot shrink or change a CIDR block** after creation, only add secondary ones
- Avoid `172.17.0.0/16` — Docker uses it by default and you will have routing conflicts
- If you ever want to peer VPCs or connect on-prem, CIDRs **must not overlap**

---

## VPC-Attached Lambda

When a Lambda function needs to talk to RDS, ElastiCache, or anything else inside a VPC, it has to run *inside* that VPC. Press `ctrl+e` to see what that looks like from the function's perspective:

```go
package main

import (
	"fmt"
	"net"
)

func main() {
	subnets := []string{"subnet-0a1b2c", "subnet-4e5f6a", "subnet-8c9d0e"}
	sg := "sg-lambda-private"

	var ip string
	addrs, _ := net.InterfaceAddrs()
	for _, a := range addrs {
		if n, ok := a.(*net.IPNet); ok && !n.IP.IsLoopback() && n.IP.To4() != nil {
			ip = n.IP.String()
			break
		}
	}

	fmt.Println("Lambda VPC Configuration")
	fmt.Printf("  Private IP:      %s\n", ip)
	fmt.Printf("  Subnets:         %v\n", subnets)
	fmt.Printf("  Security Group:  %s\n", sg)
}
```

---

## Security Groups

Security groups are **stateful firewalls** attached to elastic network interfaces. If you allow traffic in, the response is automatically allowed out.

- **Allow rules only** — there is no way to write an explicit deny
- All rules are evaluated together; if any rule matches, traffic is allowed
- You can reference another security group as a source instead of an IP range
- Default: deny everything inbound, allow everything outbound
- Limit: 5 SGs per ENI, 60 inbound + 60 outbound rules each (adjustable to 200)

A practical example for a web application:

| SG Name        | Inbound                          | Outbound     |
|----------------|----------------------------------|--------------|
| `sg-alb`       | `0.0.0.0/0` on 443              | `sg-app`:8080|
| `sg-app`       | `sg-alb` on 8080                | `sg-db`:5432, `0.0.0.0/0`:443 |
| `sg-db`        | `sg-app` on 5432                | deny all     |

Each tier only accepts traffic from the tier above it. No IP addresses, just security group references. If an instance moves, the rules follow automatically.

---

## NACLs — When You Need Them

Network ACLs are the **stateless** firewall at the subnet level. Most teams never touch them, and that is fine. But they exist for a reason.

- Rules have **priority numbers** — lowest number wins, first match applies
- Support both **allow and deny** — the only way to explicitly block an IP in AWS networking
- **Stateless** means you need rules in both directions: if you allow inbound 443, you must also allow outbound ephemeral ports (1024-65535) for the reply
- One NACL per subnet, default NACL allows everything

When we actually use NACLs:

- **Blocking a known-bad IP range** — security groups can't deny, NACLs can
- **Compliance requirement** that demands subnet-level filtering in addition to instance-level
- **Defense in depth** — NACLs as a coarse outer layer, SGs as the fine-grained inner layer

If you don't have one of those reasons, stick with security groups. They are easier to reason about because they are stateful.

---

## Elastic Load Balancing

All three flavors sit in your public subnets, accept traffic, and forward it to targets in private subnets.

| Type    | OSI Layer | Protocols       | Key Strength                          |
|---------|-----------|-----------------|---------------------------------------|
| **ALB** | 7         | HTTP, HTTPS, gRPC | Content-based routing, sticky sessions|
| **NLB** | 4         | TCP, UDP, TLS   | Millions of RPS, static IPs, <100µs latency |
| **GLB** | 3         | IP packets      | Inline appliances: firewalls, IDS     |

### ALB specifics

- Route by **host header**, **path**, **query string**, or **HTTP method**
- **Weighted target groups** for canary deploys — send 5% to the new version
- Native integration with **Cognito** for authentication
- **Slow start** mode — ramp traffic to new targets over 30-900 seconds

### NLB specifics

- Preserves the client's source IP (ALB does not, without X-Forwarded-For)
- Supports **Elastic IPs** — useful when clients need to allowlist a static address
- Can front an ALB if you need both static IPs and layer-7 routing
- **PrivateLink** — expose your service to other VPCs via NLB

Both support **cross-zone load balancing** and **connection draining** (deregistration delay).

---

## CloudFront

A CDN with 450+ edge locations. Not just for static files — it handles APIs, WebSocket, and video too.

### Why bother?

- **Latency** — your users hit an edge location 20ms away, not a region 200ms away
- **TLS handshake** — happens at the edge, not the origin. That alone saves a round trip.
- **Shield Standard** — free DDoS protection on every distribution, no config needed
- **Cost** — CloudFront data transfer is cheaper than direct-from-EC2 in most cases

### S3 as origin

- Use **Origin Access Control** so the bucket stays fully private — no public access at all
- CloudFront handles gzip and brotli compression automatically
- Set `Cache-Control: max-age=31536000, immutable` on hashed assets, `no-cache` on `index.html`

### ALB as origin

- Forward `Host`, `Authorization`, and any headers your app needs via an **origin request policy**
- Add a custom header (`X-CloudFront-Secret`) and validate it on the ALB — this prevents people from hitting the ALB directly
- Use **origin groups** with failover: primary ALB in eu-west-1, secondary in us-east-1

### Lambda@Edge vs CloudFront Functions

- **CloudFront Functions**: viewer request/response only, 1ms max, JS, dirt cheap
- **Lambda@Edge**: all four events, 30s max, Node/Python, more expensive but more powerful

---

## What is Route 53?

AWS runs one of the largest authoritative DNS networks in the world. Route 53 is the product name — a reference to UDP/TCP port 53.

It does three things:

1. **Domain registration** — buy `example.com` directly in the console, no third-party registrar needed. Includes WHOIS privacy by default.

2. **DNS hosting** — create hosted zones with records that resolve worldwide. Anycast network means every query hits the nearest edge location. AWS guarantees a **100% availability SLA** — the only service in their portfolio with that number.

3. **Health checks and routing** — monitor your endpoints and route traffic away from failures automatically. This is where Route 53 becomes more than just DNS.

Pricing is simple: **$0.50/month** per hosted zone, **$0.40** per million standard queries. Alias queries to AWS resources (ALB, CloudFront, S3) are **free**.

---

## DNS Records That Matter

| Type      | What It Does                                 |
|-----------|----------------------------------------------|
| **A**     | Maps a name to an IPv4 address               |
| **AAAA**  | Maps a name to an IPv6 address               |
| **CNAME** | Points a name to another name (no zone apex) |
| **Alias** | AWS-only: points a name to an AWS resource   |
| **MX**    | Routes email to a mail server                |
| **TXT**   | Arbitrary text — SPF, DKIM, domain verification |
| **SRV**   | Service discovery with port and priority     |
| **NS**    | Delegates a subdomain to other nameservers   |

The one that confuses everyone: **Alias vs CNAME**.

- A CNAME maps `www.example.com → app.example.com`. It **cannot** be used at the zone apex (`example.com` bare).
- An Alias maps `example.com → d1234.cloudfront.net`. It **can** be used at the apex, is resolved server-side by Route 53 (faster), and queries to AWS resources are **free**.

If you are pointing to an ALB, CloudFront, S3, or API Gateway — always use Alias.

---

## Routing Policies

This is where DNS becomes a traffic management tool.

**Simple** — one record, one answer. No health checks. Fine for dev environments.

**Failover** — primary/secondary with a health check. If the primary fails the check, Route 53 answers with the secondary. We use this for our status page: primary is CloudFront, secondary is an S3 static site in a different region.

**Weighted** — split traffic by percentage. We route 95% to the current release and 5% to the canary. If error rates spike, flip the weight to 0% without a deploy.

**Latency** — Route 53 maintains a latency table between its edge locations and AWS regions. A user in Tokyo gets routed to ap-northeast-1, a user in Dublin gets eu-west-1. No configuration beyond creating the records.

**Geolocation** — route by country or continent. We use this for GDPR: European users always hit `eu-west-1`, never cross the Atlantic.

**Geoproximity** — like geolocation but continuous. You set a bias value to pull or push traffic toward a region. Useful when you want "mostly nearest region" but need to drain a region for maintenance.

**Multivalue** — returns up to 8 healthy IPs. Basically poor man's load balancing at the DNS level.

---

## Reference Architecture

A real setup we run in production:

**Route 53** → **CloudFront** → **ALB** → **ECS in private subnets**

- Route 53 **Alias** record on `app.example.com` pointing to a CloudFront distribution
- CloudFront terminates TLS at the edge with an ACM certificate, caches static assets, forwards `/api/*` to the ALB
- ALB lives in three **public subnets** across `eu-west-1a/b/c`, terminates a second TLS hop with a private cert, validates the `X-CloudFront-Secret` header
- ECS Fargate tasks run in three **private subnets**, security group allows only port 8080 from `sg-alb`
- RDS Aurora in three **data subnets**, security group allows only port 5432 from `sg-app`
- **NAT Gateway** in one public subnet (we accept the single-AZ risk for cost; flip to three for critical workloads)
- **Gateway Endpoint** for S3 — all artifact pulls and log shipping stay off the NAT
- **Interface Endpoint** for ECR — container image pulls stay inside the VPC
- **Private Hosted Zone** maps `db.internal` to the Aurora writer endpoint

Total networking cost on this setup: roughly **$120/month** — one NAT GW, one ALB, CloudFront transfer, and a hosted zone.

---

## Private DNS and Hybrid Connectivity

### Private Hosted Zones

DNS that only resolves from inside your VPC:

- `api.internal → 10.0.48.10` — application tier
- `db.internal → 10.0.96.50` — Aurora writer endpoint
- `cache.internal → 10.0.96.100` — ElastiCache cluster

Associate one zone with multiple VPCs, even **cross-account**. The dev VPC and prod VPC can each resolve their own `db.internal` pointing to different targets.

### Requirements

- `enableDnsSupport = true` — turns on the VPC resolver at `VPC_CIDR + 2` (e.g. `10.0.0.2`)
- `enableDnsHostnames = true` — EC2 instances get a hostname like `ip-10-0-48-10.eu-west-1.compute.internal`

### Route 53 Resolver (hybrid DNS)

When you have an on-premises data center connected via Direct Connect or VPN:

- **Inbound endpoints** — on-prem DNS servers forward queries to your AWS private zones
- **Outbound endpoints** — VPC workloads forward queries for on-prem domains (e.g. `corp.internal`) to your DC's DNS
- **Resolver rules** — map domain suffixes to target DNS servers, share rules across accounts via RAM

---

# Thank You

### AWS Networking Deep Dive

VPC, security, load balancing, CDN, and DNS —

from CIDR blocks to production architecture.
