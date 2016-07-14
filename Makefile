#BUILD_VERBOSE :=
BUILD_VERBOSE := -v

TEST_VERBOSE :=
TEST_VERBOSE := -v

all:
	go build $(BUILD_VERBOSE)

.PHONY: docker
docker:
	CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo
	docker build -t $$USER/coredns .

.PHONY: deps
deps:
	go get ${BUILD_VERBOSE}

.PHONY: test
test:
	go test $(TEST_VERBOSE) ./...

.PHONY: testk8s
testk8s:
#	go test $(TEST_VERBOSE) -tags=k8sIntegration ./...
	go test $(TEST_VERBOSE) -tags=k8sIntegration -run 'TestK8sIntegration' ./test

.PHONY: clean
clean:
	go clean
