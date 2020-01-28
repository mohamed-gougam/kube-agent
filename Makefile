all: push

VERSION = edge
TAG = $(VERSION)
PREFIX = mgnginx/kube-agent

DOCKERFILE = build/Dockerfile

kube-agent:
	GOOS=linux GOARCH=amd64 go build -ldflags="-w -s" -o kube-agent github.com/mohamed-gougam/kube-agent/cmd/kube-agent/

verify-codegen:
	./hack/verify-codegen.sh

update-codegen:
	./hack/update-codegen.sh

container: verify-codegen kube-agent
	docker build -t $(PREFIX):$(TAG) -f $(DOCKERFILE) .

push: container
	docker push $(PREFIX):$(TAG)

clean:
	rm kube-agent
