.PHONY: test test-unit test-integration demo deploy-local linter install build client drand relay-http relay-gossip relay-s3

####################  Lint and fmt process ##################
install_lint:
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(shell go env GOPATH)/bin v1.41.1

lint: build_proto
	golangci-lint --version
	golangci-lint run -E gofmt -E gosec -E goconst -E gocritic --timeout 5m

lint-todo: build_proto
	golangci-lint run -E stylecheck -E gosec -E goconst -E godox -E gocritic

fmt: build_proto
	@echo "Checking (& upgrading) formatting of files. (if this fail, re-run until success)"
	@{ \
		files=$$( go fmt ./... ); \
		if [ -n "$$files" ]; then \
		echo "Files not properly formatted: $$files"; \
		exit 1; \
		fi; \
	}

check-modtidy:
	go mod tidy
	git diff --exit-code -- go.mod go.sum

clean:
	go clean

#################################### Testing #############################################
test: test-unit test-integration

test-unit: build_proto
	GO111MODULE=on go test -race -short -v ./...

test-unit-coverage: build_proto
	GO111MODULE=on go test -short -v -coverprofile=coverage.txt -covermode=count -coverpkg=all $(go list ./... | grep -v /demo/)

test-integration: build_proto
	go test -v ./demo
	cd demo && go build && ./demo -build -test -debug

coverage: build_proto
	go get -u github.com/ory/go-acc
	go get -v -t -d ./...
	COVERAGE=true go-acc ./...

demo: build_proto
	cd demo && go build && ./demo -build
	#cd demo && sudo ./run.sh

############################################ Build ############################################

build_proto:
	go get -u github.com/golang/protobuf/protoc-gen-go
	go get -u google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
	cd protobuf && sh ./compile_proto.sh

# create the "drand" binary and install it in $GOBIN
install: build_proto
	go install -ldflags "-X github.com/drand/drand/cmd/drand-cli.version=`git describe --tags` -X github.com/drand/drand/cmd/drand-cli.buildDate=`date -u +%d/%m/%Y@%H:%M:%S` -X github.com/drand/drand/cmd/drand-cli.gitCommit=`git rev-parse HEAD`"

# create the "drand" binary in the current folder
build: build_proto
	go build -o drand -mod=readonly -ldflags "-X github.com/drand/drand/cmd/drand-cli.version=`git describe --tags` -X github.com/drand/drand/cmd/drand-cli.buildDate=`date -u +%d/%m/%Y@%H:%M:%S` -X github.com/drand/drand/cmd/drand-cli.gitCommit=`git rev-parse HEAD`"

drand: build

# create the "drand-client" binary in the current folder
client: build_proto
	go build -o drand-client -mod=readonly -ldflags "-X main.version=`git describe --tags` -X main.buildDate=`date -u +%d/%m/%Y@%H:%M:%S` -X main.gitCommit=`git rev-parse HEAD`" ./cmd/client
drand-client: client

# create the "drand-relay-http" binary in the current folder
relay-http: build_proto
	go build -o drand-relay-http -mod=readonly -ldflags "-X main.version=`git describe --tags` -X main.buildDate=`date -u +%d/%m/%Y@%H:%M:%S` -X main.gitCommit=`git rev-parse HEAD`" ./cmd/relay

drand-relay-http: relay-http

# create the "drand-relay-gossip" binary in the current folder
relay-gossip: build_proto
	go build -o drand-relay-gossip -mod=readonly -ldflags "-X main.version=`git describe --tags` -X main.buildDate=`date -u +%d/%m/%Y@%H:%M:%S` -X main.gitCommit=`git rev-parse HEAD`" ./cmd/relay-gossip

drand-relay-gossip: relay-gossip

# create the "drand-relay-s3" binary in the current folder
relay-s3: build_proto
	go build -o drand-relay-s3 -mod=readonly -ldflags "-X main.version=`git describe --tags` -X main.buildDate=`date -u +%d/%m/%Y@%H:%M:%S` -X main.gitCommit=`git rev-parse HEAD`" ./cmd/relay-s3

drand-relay-s3: relay-s3


############################################ Deps ############################################

install_deps_linux:
	PROTOC_ZIP=protoc-3.14.0-linux-x86_64.zip
	curl -OL https://github.com/protocolbuffers/protobuf/releases/download/v3.14.0/$PROTOC_ZIP
	sudo unzip -o $PROTOC_ZIP -d /usr/local bin/protoc
	sudo unzip -o $PROTOC_ZIP -d /usr/local 'include/*'
	rm -f $PROTOC_ZIP

install_deps_darwin:
	brew install protobuf