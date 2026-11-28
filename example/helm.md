---
author: levi van noort
date: YYYY-MM-dd
paging: "%d / %d"
align: center
theme: kontrolplane
headingColor: "#F4E8C1"
execTimeout: 5m
---

# helm

Writing a chart, then installing, changing and undoing it.

```properties
needs   docker, k3d, kubectl and helm
keys    ctrl+e runs a command, tab picks another on the same slide
```

---

## Manifests on their own

`kubectl apply` works for one manifest. An application has many.

```properties
copies    staging and production differ in three lines of two files
pieces    twenty objects are one application, and nothing says so
history   apply overwrites: what ran before is kept nowhere
```

> Helm has an answer to each: templates, releases and revisions.

---

## A chart is a directory

```bash
mkdir -p /tmp/lekture/web/templates && cd /tmp/lekture/web
cat > Chart.yaml <<EOF
apiVersion: v2
name: web
version: 0.1.0
EOF
cat > values.yaml <<EOF
replicas: 2
tag: 6.16.0
EOF
ls
```

```text
web
├── Chart.yaml     the chart's name and its own version
├── values.yaml    the settings, each with a default
└── templates      manifests with gaps where the settings go
```

```properties
tag   which version to run of podinfo, a small web app
```

---

## A template

```bash
cat > /tmp/lekture/web/templates/deployment.yaml <<EOF
apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ .Release.Name }}
spec:
  replicas: {{ .Values.replicas }}
  selector:
    matchLabels:
      app: {{ .Release.Name }}
  template:
    metadata:
      labels:
        app: {{ .Release.Name }}
    spec:
      containers:
        - name: app
          image: ghcr.io/stefanprodan/podinfo:{{ .Values.tag }}
EOF
helm lint /tmp/lekture/web
```

> An ordinary manifest with three things left open.

---

## Filling the gaps

```bash
helm template demo /tmp/lekture/web
```

```bash
helm template demo /tmp/lekture/web --set replicas=5 | grep replicas
```

```properties
template   prints the finished manifests and changes nothing
demo       a release name: it goes where .Release.Name stands
.Values    read from values.yaml, unless you give another value
--set      gives another value, for this one command
```

---

## Who decides a value

```text
┌─ the chart ───────┐   ┌─ your file ───────┐   ┌─ a flag ──────────┐
│ values.yaml       │   │ -f values.yaml    │   │ --set replicas=5  │
│ replicas: 2       │ ▶ │ replicas: 3       │ ▶ │                   │
│ every default     │   │ what differs      │   │ a one-off         │
└───────────────────┘   └───────────────────┘   └───────────────────┘
```

```bash
helm template demo /tmp/lekture/web --set replicas=5 \
  -f - <<EOF | grep replicas
replicas: 3
EOF
```

> Each one overrides the one to its left.

---

## A cluster to install into

```bash
k3d cluster create lekture --agents 2 \
  --kubeconfig-switch-context=false
```

```properties
k3d       runs Kubernetes inside Docker: each node is a container
--kube…   leaves the cluster your kubectl points at unchanged
skip      this, when example/kubernetes.md left its cluster running
```

---

## Install

```bash
helm install demo /tmp/lekture/web --kube-context k3d-lekture
```

```bash
helm list --kube-context k3d-lekture
```

```bash
kubectl --context k3d-lekture get deployments,pods
```

```properties
release   one install of a chart, under a name you choose
demo      that name: a second install needs a second name
```

---

## Upgrade

```bash
helm upgrade demo /tmp/lekture/web --kube-context k3d-lekture \
  --set replicas=4 --set tag=6.17.0
```

```bash
helm history demo --kube-context k3d-lekture
```

```bash
kubectl --context k3d-lekture get pods -o \
  'custom-columns=POD:.metadata.name,IMAGE:.spec.containers[*].image'
```

> Every upgrade is a new revision. The earlier ones are kept.

---

## Rollback

```bash
helm rollback demo 1 --kube-context k3d-lekture
```

```bash
kubectl --context k3d-lekture get pods -o \
  'custom-columns=POD:.metadata.name,IMAGE:.spec.containers[*].image'
```

```bash
kubectl --context k3d-lekture get secrets -l owner=helm
```

```properties
revision   one entry in a release's history: an install or an upgrade
secrets    where Helm keeps each revision: in the cluster itself
rollback   a third revision, made from the manifests of the first
```

---

## Package

```bash
helm package /tmp/lekture/web -d /tmp/lekture
```

```bash
helm show chart /tmp/lekture/web-0.1.0.tgz
```

```properties
package   the chart as one file, named by its name and version
push      helm push sends that file to a registry, as with an image
install   takes a directory, a file or a registry address alike
```

---

## Cleanup

```bash
helm uninstall demo --kube-context k3d-lekture
```

```bash
k3d cluster delete lekture
```

```bash
rm -r /tmp/lekture
```

---

# thank you

Questions?

```properties
docs      helm.sh/docs
charts    artifacthub.io, for charts that others have published
starter   helm create writes a fuller chart to start from
```
