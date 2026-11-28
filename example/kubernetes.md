---
author: levi van noort
date: YYYY-MM-dd
paging: "%d / %d"
align: center
theme: kontrolplane
headingColor: "#F4E8C1"
execTimeout: 5m
---

# kubernetes

From one container to a cluster that looks after itself.

```properties
needs   docker, k3d and kubectl
keys    ctrl+e runs a command, tab picks another on the same slide
```

---

## Containers need looking after

A container is one process. Somebody has to mind it.

```properties
crash      it stops, and stays stopped until someone starts it again
traffic    one copy is not enough, so more have to be started
hardware   a machine fails and takes its containers with it
release    a new version has to replace the old one without a gap
```

> Kubernetes takes over these four jobs.

---

## A cluster

A set of machines that Kubernetes runs as one.

```text
┌─ control plane ───────────────────────────────────────────────────┐
│         api server  ·  scheduler  ·  controllers  ·  etcd         │
└─────────┬───────────────────────┬───────────────────────┬─────────┘
┌─ node ──┴─────────┐   ┌─ node ──┴─────────┐   ┌─ node ──┴─────────┐
│ kubelet           │   │ kubelet           │   │ kubelet           │
│ ■ pod  ■ pod      │   │ ■ pod             │   │ ■ pod  ■ pod      │
└───────────────────┘   └───────────────────┘   └───────────────────┘
```

```properties
control-plane   takes your requests and decides what runs where
node            a machine that runs containers
kubelet         the program on a node that starts and stops them
pod             what it starts: one or more containers, one address
```

---

## Start one

```bash
k3d cluster create lekture --agents 2 \
  --kubeconfig-switch-context=false
```

```bash
kubectl --context k3d-lekture get nodes
```

```properties
k3d         runs Kubernetes inside Docker: each node is a container
--agents    two more nodes, beside the one with the control plane
--kube…     leaves the cluster your kubectl points at unchanged
--context   names the cluster, so no command lands on another one
```

---

## A pod on its own

Start one pod, list it, delete it, then list again.

```bash
kubectl --context k3d-lekture run web \
  --image=ghcr.io/stefanprodan/podinfo:6.16.0
```

```bash
kubectl --context k3d-lekture get pods
```

```bash
kubectl --context k3d-lekture delete pod web
```

```properties
podinfo   the image: a small web app made for trying things out
```

> It is gone for good. Nobody was asked to keep it running.

---

## Ask for an outcome

```bash
kubectl --context k3d-lekture apply -f - <<EOF
apiVersion: apps/v1
kind: Deployment
metadata:
  name: web
spec:
  replicas: 3
  selector:
    matchLabels:
      app: web
  template:
    metadata:
      labels:
        app: web
    spec:
      containers:
        - name: app
          image: ghcr.io/stefanprodan/podinfo:6.16.0
EOF
```

> Not "start a pod" but "there are three pods like this one".

---

## What the manifest says

```bash
kubectl --context k3d-lekture get deployments,pods
```

```properties
manifest     a description of one object, written as YAML
deployment   the object that keeps a number of equal pods running
replicas     that number
template     what each of those pods looks like
labels       tags on the pods: here app=web
selector     how the deployment finds its pods: by that tag
```

---

## The loop that keeps it true

```text
┌─ look ──────────┐     ┌─ compare ───────┐     ┌─ act ───────────┐
│ how many pods   │ ──▶ │ running: 2      │ ──▶ │ start one more  │
│ are running?    │     │ wanted:  3      │     │ pod             │
└────────▲────────┘     └─────────────────┘     └────────┬────────┘
         └─────────────── and look again ────────────────┘
```

> It never stops. Kubernetes calls a loop like this a controller.

---

## Break it

Delete all three pods, then list them.

```bash
kubectl --context k3d-lekture delete pods -l app=web --wait=false
```

```bash
kubectl --context k3d-lekture get pods
```

> Three are back under new names. AGE shows they have just started.

---

## More copies

```bash
kubectl --context k3d-lekture scale deployment web --replicas=6
```

```bash
kubectl --context k3d-lekture get pods \
  -o custom-columns=POD:.metadata.name,NODE:.spec.nodeName
```

```properties
scale       changes one number in the deployment: replicas
scheduler   picks a node for each new pod and spreads them out
```

---

## One address for all of them

Pods come and go, and their addresses with them. A service stays.

```bash
kubectl --context k3d-lekture expose deployment web \
  --port 80 --target-port 9898
```

```bash
kubectl --context k3d-lekture get endpointslices \
  -l kubernetes.io/service-name=web
```

```bash
kubectl --context k3d-lekture run client --rm -i --restart=Never \
  --image=curlimages/curl -- curl -s web | grep -E 'hostname|version'
```

```properties
service   one name in front of every pod that has app=web
client    a pod that asks that name for a page, then goes away
podinfo   the app in the pods: it answers with its name and version
```

---

## A new version

Change the image and watch the pods being replaced.

```bash
kubectl --context k3d-lekture set image deployment web \
  app=ghcr.io/stefanprodan/podinfo:6.17.0
```

```bash
kubectl --context k3d-lekture rollout status deployment web
```

```bash
kubectl --context k3d-lekture run client --rm -i --restart=Never \
  --image=curlimages/curl -- curl -s web | grep -E 'hostname|version'
```

> Old pods leave as new ones are ready. `rollout undo` goes back.

---

## Cleanup

```bash
k3d cluster delete lekture
```

<!-- pause -->

```properties
next   example/helm.md packages this deployment as a chart
```

---

# thank you

Questions?

```properties
docs       kubernetes.io/docs
kubectl    kubernetes.io/docs/reference/kubectl
k3d        k3d.io
left-out   namespaces, ingress, configuration and storage
```
