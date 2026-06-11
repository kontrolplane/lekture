---
author: levi van noort
date: YYYY-MM-dd
paging: "%d / %d"
---

# AWS Networking Deep Dive

### VPC, Route 53, CloudFront & ELB

---

## Agenda

1. **VPC** — your private network in the cloud
2. **Security** — security groups, NACLs, traffic filtering
3. **Load Balancing** — ALB, NLB, and GLB
4. **CloudFront** — edge caching and content delivery
5. **Route 53** — DNS, health checks, routing policies
6. **Reference Architecture** — tying it all together

> Real CIDR examples, production gotchas, and cost breakdowns throughout.

---

## What is a VPC?

Your own logically isolated network inside AWS. Nothing gets in or out unless you allow it.

- Scoped to one **region**, spans all its **availability zones**
- You pick the **CIDR block**, carve **subnets**, wire up **route tables**
- Every account gets a default VPC — **never** use it for real workloads
- Dual-stack: **IPv4**, **IPv6**, or both at once

The VPC itself is free. You pay for what you attach: NAT Gateways, VPN tunnels, Transit Gateway, cross-AZ transfer.

> An intern once exposed an RDS instance on the default VPC. A custom VPC with no IGW on its private subnets would have prevented it entirely.

---

## VPC Building Blocks

| Component            | Role                                       |
|----------------------|--------------------------------------------|
| **CIDR Block**       | IP range, e.g. `10.0.0.0/16`               |
| **Subnet**           | A slice of the range, pinned to one AZ     |
| **Route Table**      | Decides where packets go next              |
| **Internet Gateway** | Two-way door to the internet               |
| **NAT Gateway**      | Outbound-only door for private subnets     |
| **Security Group**   | Stateful firewall, per-interface           |
| **NACL**             | Stateless firewall, per-subnet             |
| **VPC Endpoint**     | Private path to AWS services               |
| **Peering**          | Direct 1:1 VPC link (non-transitive)       |
| **Transit Gateway**  | Hub-and-spoke router for many VPCs         |

> **Watch the NAT bill** — `$0.045/hr` + `$0.045/GB`. A busy subnet runs hundreds a month. Gateway Endpoints for S3 and DynamoDB are free.

---

## Subnets in Practice

**Public subnet** — default route `0.0.0.0/0` points to an Internet Gateway.

- Hosts ALBs, bastion hosts, NAT Gateways
- Instances need a public or Elastic IP to be reachable
- Set `MapPublicIpOnLaunch` so you don't forget per-instance

**Private subnet** — default route goes to a NAT Gateway, or nowhere.

- Hosts app servers, databases, Lambda, ECS tasks
- Reaches the internet outbound via NAT for patches and APIs
- Use free **Gateway Endpoints** for S3/DynamoDB

> Run **three of each** across `eu-west-1a/b/c`. The ALB needs two AZs minimum — three gives every tier AZ-level fault tolerance.

---

## CIDR Planning

Rush this and it bites you later.

| Block           | Size   | Used For                  |
|-----------------|--------|---------------------------|
| `10.0.0.0/16`   | 65,536 | The VPC itself            |
| `10.0.0.0/20`   | 4,096  | Public subnets            |
| `10.0.48.0/20`  | 4,096  | Private subnets           |
| `10.0.96.0/20`  | 4,096  | Data tier (RDS, cache)    |
| `10.0.144.0/20` | 4,096  | Spare — future growth     |

- AWS reserves **5 IPs** per subnet (`.0`–`.3` and the last)
- Ranges span `/16` down to `/28` — nothing larger
- **You can't shrink or change** a CIDR after creation
- Avoid `172.17.0.0/16` — Docker's default, it conflicts
- Peering or going on-prem? CIDRs **must not overlap**

---

## VPC-Attached Lambda

To reach RDS, ElastiCache, or anything inside a VPC, a Lambda has to run *inside* it. Press `ctrl+e` to see it from the function's perspective:

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

**Stateful** firewalls on each network interface. Allow traffic in, and the reply is allowed back out automatically.

- **Allow rules only** — there is no explicit deny
- Reference another security group as a source, not just IPs
- Default: deny all inbound, allow all outbound
- Limit: 5 SGs per ENI, 60 rules each (raisable to 200)

| SG        | Inbound          | Outbound              |
|-----------|------------------|-----------------------|
| `sg-alb`  | `0.0.0.0/0`:443  | `sg-app`:8080         |
| `sg-app`  | `sg-alb`:8080    | `sg-db`:5432, `:443`  |
| `sg-db`   | `sg-app`:5432    | —                     |

> Each tier accepts traffic only from the tier above it — by SG reference, not IP. Move an instance and the rules follow it.

---

## NACLs — When You Need Them

The **stateless** firewall at the subnet level. Most teams never touch them, and that's fine.

- Rules have **priority numbers** — lowest wins, first match applies
- Support **allow *and* deny** — the only way to block an IP in AWS
- Stateless: allow inbound 443 *and* outbound ephemeral ports (1024–65535) for the reply
- One per subnet; the default allows everything

Reach for them only when:

- **Blocking a bad IP range** — SGs can't deny, NACLs can
- **Compliance** demands subnet-level filtering
- **Defense in depth** — a coarse outer layer over SGs

> No such reason? Stick with security groups. Stateful is far easier to reason about.

---

## Elastic Load Balancing

All three sit in your public subnets and forward to private targets.

| Type    | Layer | Protocols       | Strength                        |
|---------|-------|-----------------|---------------------------------|
| **ALB** | 7     | HTTP(S), gRPC   | Content routing, sticky sessions|
| **NLB** | 4     | TCP, UDP, TLS   | Millions of RPS, static IPs     |
| **GLB** | 3     | IP packets      | Inline firewalls, IDS           |

### ALB

- Route by **host**, **path**, **query**, or **method**
- **Weighted target groups** for canary deploys
- Native **Cognito** auth and **slow start** ramping

### NLB

- Preserves client source IP; supports **Elastic IPs**
- Can front an ALB for static IPs *and* L7 routing
- **PrivateLink** — expose a service to other VPCs

> Both do cross-zone balancing and connection draining.

---

## CloudFront

A CDN with 450+ edge locations — not just static files, but APIs, WebSocket, and video too.

### Why bother

- **Latency** — users hit an edge 20ms away, not a region 200ms away
- **TLS** terminates at the edge, saving a full round trip
- **Shield Standard** — free DDoS protection, zero config
- **Cost** — edge transfer beats direct-from-EC2

### S3 origin

- **Origin Access Control** keeps the bucket fully private
- Automatic gzip and brotli compression
- `max-age=31536000, immutable` on hashed assets

### ALB origin

- Forward needed headers via an **origin request policy**
- Validate a secret header so nobody hits the ALB directly
- **Origin groups** for cross-region failover

### Functions vs Lambda@Edge

- **CloudFront Functions** — viewer events, 1ms, JS, dirt cheap
- **Lambda@Edge** — all events, 30s, Node/Python, powerful

---

## What is Route 53?

AWS runs one of the world's largest authoritative DNS networks. Named for port 53.

1. **Domain registration** — buy domains in-console, WHOIS privacy included
2. **DNS hosting** — anycast hosted zones with a **100% availability SLA** (their only one)
3. **Health checks & routing** — steer traffic away from failures automatically

> **$0.50/mo** per hosted zone, **$0.40** per million queries. Alias queries to AWS resources are **free**.

---

## DNS Records That Matter

| Type            | What It Does                          |
|-----------------|---------------------------------------|
| **A** / **AAAA**| Name → IPv4 / IPv6 address            |
| **CNAME**       | Name → another name (not at apex)     |
| **Alias**       | AWS-only: name → an AWS resource      |
| **MX**          | Routes email to a mail server         |
| **TXT**         | SPF, DKIM, domain verification        |
| **SRV**         | Service discovery (port + priority)   |
| **NS**          | Delegates a subdomain                 |

**Alias vs CNAME** — the one everyone trips on:

- **CNAME** maps name → name, and **cannot** sit at the zone apex
- **Alias** maps the apex → an AWS resource, resolves server-side (faster), and is **free**

> Pointing at an ALB, CloudFront, S3, or API Gateway? Always use Alias.

---

## Routing Policies

DNS as a traffic-management tool.

- **Simple** — one record, one answer, no health checks
- **Failover** — primary/secondary on a health check
- **Weighted** — split by percentage; 95% release, 5% canary
- **Latency** — route to the lowest-latency region automatically
- **Geolocation** — route by country or continent (great for GDPR)
- **Geoproximity** — continuous, with a bias to drain regions
- **Multivalue** — up to 8 healthy IPs; DNS-level balancing

> Flip a canary's weight to 0% the instant error rates spike — no deploy needed.

---

## Reference Architecture

**Route 53 → CloudFront → ALB → ECS** in private subnets.

- **Route 53** Alias on `app.example.com` → the CloudFront distribution
- **CloudFront** terminates TLS at the edge, caches assets, forwards `/api/*`
- **ALB** in 3 public subnets, validates the `X-CloudFront-Secret` header
- **ECS Fargate** in 3 private subnets — port 8080 from `sg-alb` only
- **RDS Aurora** in 3 data subnets — port 5432 from `sg-app` only
- **NAT Gateway** in one AZ (cost trade-off; three for critical workloads)
- **Gateway Endpoint** (S3) + **Interface Endpoint** (ECR) keep traffic off NAT
- **Private Hosted Zone** maps `db.internal` to the Aurora writer

> Total networking cost on this setup: roughly **$120/month**.

---

## Private DNS & Hybrid Connectivity

### Private Hosted Zones

DNS that resolves only from inside your VPC:

- `api.internal → 10.0.48.10`
- `db.internal → 10.0.96.50`
- `cache.internal → 10.0.96.100`

Associate one zone with multiple VPCs, even **cross-account**.

### Requirements

- `enableDnsSupport` — VPC resolver at `VPC_CIDR + 2` (e.g. `10.0.0.2`)
- `enableDnsHostnames` — instances get internal DNS names

### Route 53 Resolver (hybrid)

- **Inbound** — on-prem forwards queries into your AWS zones
- **Outbound** — VPC forwards `corp.internal` to your DC
- **Resolver rules** — map suffixes to DNS servers, share via RAM

---

# Thank You

### AWS Networking Deep Dive

![levi van noort](image.png)

VPC · security · load balancing · CDN · DNS
