# kube-agent
A simple agent in Go that monitors special "TCPServer" custom resources in Kubernetes cluster, converts those resources into NGINX configuration and applies that configuration to NGINX. We can think of this agent as a simple Controller that only understands that special custom resource.
