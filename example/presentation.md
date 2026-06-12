---
author: levi van noort
date: YYYY-MM-dd
paging: "%d / %d"
---

# aws networking deep dive

### vpc, route 53, cloudfront & elb

---

## agenda

1. **vpc** — your private network in the cloud
2. **security** — security groups, nacls, traffic filtering
3. **load balancing** — alb, nlb, and glb
4. **cloudfront** — edge caching and content delivery
5. **route 53** — dns, health checks, routing policies
6. **reference architecture** — tying it all together

> real cidr examples, production gotchas, and cost breakdowns throughout.

---

## what is a vpc?

your own logically isolated network inside aws. nothing gets in or out unless you allow it.

- scoped to one **region**, spans all its **availability zones**
- you pick the **cidr block**, carve **subnets**, wire up **route tables**
- every account gets a default vpc — **never** use it for real workloads
- dual-stack: **ipv4**, **ipv6**, or both at once

the vpc itself is free. you pay for what you attach: nat gateways, vpn tunnels, transit gateway, cross-az transfer.

> an intern once exposed an rds instance on the default vpc. a custom vpc with no igw on its private subnets would have prevented it entirely.

---

## vpc building blocks

| component            | role                                       |
|----------------------|--------------------------------------------|
| **cidr block**       | ip range, e.g. `10.0.0.0/16`               |
| **subnet**           | a slice of the range, pinned to one az     |
| **route table**      | decides where packets go next              |
| **internet gateway** | two-way door to the internet               |
| **nat gateway**      | outbound-only door for private subnets     |
| **security group**   | stateful firewall, per-interface           |
| **nacl**             | stateless firewall, per-subnet             |
| **vpc endpoint**     | private path to aws services               |
| **peering**          | direct 1:1 vpc link (non-transitive)       |
| **transit gateway**  | hub-and-spoke router for many vpcs         |

> **watch the nat bill** — `$0.045/hr` + `$0.045/GB`. a busy subnet runs hundreds a month. gateway endpoints for s3 and dynamodb are free.

---

## subnets in practice

**public subnet** — default route `0.0.0.0/0` points to an internet gateway.

- hosts albs, bastion hosts, nat gateways
- instances need a public or elastic ip to be reachable
- set `MapPublicIpOnLaunch` so you don't forget per-instance

**private subnet** — default route goes to a nat gateway, or nowhere.

- hosts app servers, databases, lambda, ecs tasks
- reaches the internet outbound via nat for patches and apis
- use free **gateway endpoints** for s3/dynamodb

> run **three of each** across `eu-west-1a/b/c`. the alb needs two azs minimum — three gives every tier az-level fault tolerance.

---

## cidr planning

rush this and it bites you later.

| block           | size   | used for                  |
|-----------------|--------|---------------------------|
| `10.0.0.0/16`   | 65,536 | the vpc itself            |
| `10.0.0.0/20`   | 4,096  | public subnets            |
| `10.0.48.0/20`  | 4,096  | private subnets           |
| `10.0.96.0/20`  | 4,096  | data tier (rds, cache)    |
| `10.0.144.0/20` | 4,096  | spare — future growth     |

- aws reserves **5 ips** per subnet (`.0`–`.3` and the last)
- ranges span `/16` down to `/28` — nothing larger
- **you can't shrink or change** a cidr after creation
- avoid `172.17.0.0/16` — docker's default, it conflicts
- peering or going on-prem? cidrs **must not overlap**

---

## vpc-attached lambda

to reach rds, elasticache, or anything inside a vpc, a lambda has to run *inside* it. press `ctrl+e` to see it from the function's perspective:

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

	fmt.Println("lambda vpc configuration")
	fmt.Printf("   private ip:      %s\n", ip)
	fmt.Printf("   subnets:         %v\n", subnets)
	fmt.Printf("   security group:  %s\n", sg)
}
```

---

## security groups

**stateful** firewalls on each network interface. allow traffic in, and the reply is allowed back out automatically.

- **allow rules only** — there is no explicit deny
- reference another security group as a source, not just ips
- default: deny all inbound, allow all outbound
- limit: 5 sgs per eni, 60 rules each (raisable to 200)

| sg        | inbound          | outbound              |
|-----------|------------------|-----------------------|
| `sg-alb`  | `0.0.0.0/0`:443  | `sg-app`:8080         |
| `sg-app`  | `sg-alb`:8080    | `sg-db`:5432, `:443`  |
| `sg-db`   | `sg-app`:5432    | —                     |

> each tier accepts traffic only from the tier above it — by sg reference, not ip. move an instance and the rules follow it.

---

## nacls — when you need them

the **stateless** firewall at the subnet level. most teams never touch them, and that's fine.

- rules have **priority numbers** — lowest wins, first match applies
- support **allow *and* deny** — the only way to block an ip in aws
- stateless: allow inbound 443 *and* outbound ephemeral ports (1024–65535) for the reply
- one per subnet; the default allows everything

reach for them only when:

- **blocking a bad ip range** — sgs can't deny, nacls can
- **compliance** demands subnet-level filtering
- **defense in depth** — a coarse outer layer over sgs

> no such reason? stick with security groups. stateful is far easier to reason about.

---

## elastic load balancing

all three sit in your public subnets and forward to private targets.

| type    | layer | protocols       | strength                        |
|---------|-------|-----------------|---------------------------------|
| **alb** | 7     | http(s), grpc   | content routing, sticky sessions|
| **nlb** | 4     | tcp, udp, tls   | millions of rps, static ips     |
| **glb** | 3     | ip packets      | inline firewalls, ids           |

### alb

- route by **host**, **path**, **query**, or **method**
- **weighted target groups** for canary deploys
- native **cognito** auth and **slow start** ramping

### nlb

- preserves client source ip; supports **elastic ips**
- can front an alb for static ips *and* l7 routing
- **privatelink** — expose a service to other vpcs

> both do cross-zone balancing and connection draining.

---

## cloudfront

a cdn with 450+ edge locations — not just static files, but apis, websocket, and video too.

### why bother

- **latency** — users hit an edge 20ms away, not a region 200ms away
- **tls** terminates at the edge, saving a full round trip
- **shield standard** — free ddos protection, zero config
- **cost** — edge transfer beats direct-from-ec2

### s3 origin

- **origin access control** keeps the bucket fully private
- automatic gzip and brotli compression
- `max-age=31536000, immutable` on hashed assets

### alb origin

- forward needed headers via an **origin request policy**
- validate a secret header so nobody hits the alb directly
- **origin groups** for cross-region failover

### functions vs lambda@edge

- **cloudfront functions** — viewer events, 1ms, js, dirt cheap
- **lambda@edge** — all events, 30s, node/python, powerful

---

## what is route 53?

aws runs one of the world's largest authoritative dns networks. named for port 53.

1. **domain registration** — buy domains in-console, whois privacy included
2. **dns hosting** — anycast hosted zones with a **100% availability sla** (their only one)
3. **health checks & routing** — steer traffic away from failures automatically

> **$0.50/mo** per hosted zone, **$0.40** per million queries. alias queries to aws resources are **free**.

---

## dns records that matter

| type            | what it does                          |
|-----------------|---------------------------------------|
| **a** / **aaaa**| name → ipv4 / ipv6 address            |
| **cname**       | name → another name (not at apex)     |
| **alias**       | aws-only: name → an aws resource      |
| **mx**          | routes email to a mail server         |
| **txt**         | spf, dkim, domain verification        |
| **srv**         | service discovery (port + priority)   |
| **ns**          | delegates a subdomain                 |

**alias vs cname** — the one everyone trips on:

- **cname** maps name → name, and **cannot** sit at the zone apex
- **alias** maps the apex → an aws resource, resolves server-side (faster), and is **free**

> pointing at an alb, cloudfront, s3, or api gateway? always use alias.

---

## routing policies

dns as a traffic-management tool.

- **simple** — one record, one answer, no health checks
- **failover** — primary/secondary on a health check
- **weighted** — split by percentage; 95% release, 5% canary
- **latency** — route to the lowest-latency region automatically
- **geolocation** — route by country or continent (great for gdpr)
- **geoproximity** — continuous, with a bias to drain regions
- **multivalue** — up to 8 healthy ips; dns-level balancing

> flip a canary's weight to 0% the instant error rates spike — no deploy needed.

---

## reference architecture

**route 53 → cloudfront → alb → ecs** in private subnets.

- **route 53** alias on `app.example.com` → the cloudfront distribution
- **cloudfront** terminates tls at the edge, caches assets, forwards `/api/*`
- **alb** in 3 public subnets, validates the `X-CloudFront-Secret` header
- **ecs fargate** in 3 private subnets — port 8080 from `sg-alb` only
- **rds aurora** in 3 data subnets — port 5432 from `sg-app` only
- **nat gateway** in one az (cost trade-off; three for critical workloads)
- **gateway endpoint** (s3) + **interface endpoint** (ecr) keep traffic off nat
- **private hosted zone** maps `db.internal` to the aurora writer

> total networking cost on this setup: roughly **$120/month**.

---

## private dns & hybrid connectivity

### private hosted zones

dns that resolves only from inside your vpc:

- `api.internal → 10.0.48.10`
- `db.internal → 10.0.96.50`
- `cache.internal → 10.0.96.100`

associate one zone with multiple vpcs, even **cross-account**.

### requirements

- `enableDnsSupport` — vpc resolver at `VPC_CIDR + 2` (e.g. `10.0.0.2`)
- `enableDnsHostnames` — instances get internal dns names

### route 53 resolver (hybrid)

- **inbound** — on-prem forwards queries into your aws zones
- **outbound** — vpc forwards `corp.internal` to your dc
- **resolver rules** — map suffixes to dns servers, share via ram

---

# thank you

### aws networking deep dive

![levi van noort](image.png)

vpc · security · load balancing · cdn · dns
