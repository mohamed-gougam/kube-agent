# kube-agent
A simple agent in Go that monitors special "TCPServer" custom resources in Kubernetes cluster, converts those resources into NGINX configuration and applies that configuration to NGINX. We can think of this agent as a simple Controller that only understands that special custom resource.

# Installing and Testing the kube-agent
This explains how to test kube-agent. This repo contains also a simple NGINX tcp server app that we will use as our service in kubernetes, and the kube-agent will load balance traffic to it.

Before following the instructions below, clone this repo locally:

```
$ git clone https://github.com/mohamed-gougam/kube-agent.git
```

## 1. Build and push kube-agent image

## 1.1 Makefile

Makefile targets:
- **kube-agent:** runs Go build and results kube-agent binary file.
- **update-codegen:** updates the generated kubernetes client code.
- **verify-codegen:** verifies that the actual generated code is up to date.
- **container:** builds the docker image locally after veifying the generated code and building kube-agent.
- **push:** pushes the image to registry after building it.
- **clean:** cleans the repo by deleting the binary kube-agent file.

```
$ cd kube-agent
```

Edit PREFIX, TAG and VERSION variables in the makefile to match your docker registry.
```
$ make
```

An image is already built and can be pulled from `mgnginx/kube-agent:edge`, it is the image we are using to deploy the kube-agent in kubernetes.

## 2. Deploy the kube-agent

```
$ cd deployments

$ kubectl apply -f common/tcpserver.crd

$ kubectl apply common/ns-and-sa.yaml

$ kubectl apply rbac/rbac.yaml

$ kubectl apply -f deployment/kube-agent.yaml
```

We can check if the agent is deployed without problem:
```
$ kubctl -n kube-agent get pods
```

## 3. Access the kube-agent

Create a service of type NodePort, here we are exposing ports 80, 443 (will serve with NGINX first install config) and port 8888 to test a simple tcp server later:
```
$ kubectl apply -f service/nodeport.yaml

$ kubectl -n kube-agent get svc
```
Take note of the corresponding port binding port 8888.

## 4. Create and test the TCPServer resource

### 4.1 TCPServer

We can create either the TCPServer first or the service that the TCPServer will reference first. If we create the TCPServer with spec.serviceName of a non existant service (or a service that has no endpoints) in the TCPServer's namespace, kube-agent will serve time on spec.listenPort with background port 37 (NGINX side).

Create the first TCPServer object:
```
$ cd examples/tcpserver-example

$ kubectl apply -f tcpserver-lb-coffee.yaml
```

Since no service exist with the name `tcpserver-coffee-svc` in the namespace `default` kube-agent will serve time in the port `8888`.
```
$ nc <kube-agent-node-IP> <node-8888-binded-port>
```

Result should be:
```
2020-01-27T01:14:53+00:00
```

### 4.2 Service

We created a simple NGINX webserver. See `examples/tcpserver-example/Dockerfile` and the `.conf` files.

Build and push the image to your repo or use `mgnginx/tcpserver-example:latest` image.

**Note:**
If you are using your own image, make sure to edit the tcpserver-coffee.yaml file.

Create the service `tcpserver-coffee-svc`:
```
$ kubectl apply -f tcpserver-coffee.yaml
```

The kube-agent will now serve `tcpserver-coffee-svc` on listenPort:
```
$ nc <kube-agent-node-IP> <node-8888-binded-port>
```

Result should be:
```
Server address: 172.17.0.4:12345
Server name: tcpserver-coffee-d67db4598-772vl
Date: 27/Jan/2020:01:15:18 +0000
Connection serial/server: 1
```

### 4.3 Scaling

Check that the kube-agent will be load balancing traffic correctly between the endpoints of the service after scaling up or down the deployment pods replicas:

```
kubectl scale --replicas=3 deployment/tcpserver-coffee
```

Kube-agent should've correctly reconfigured to load balance between the updated endpoints.