.PHONY: native
native:
	go build -v -o matrix

.PHONY: arm
arm:
	GOOS=linux GOARCH=arm GOARM=5 go build -v -o matrix.arm

.PHONY: amd64
amd64:
	GOOS=linux GOARCH=amd64 go build -v -o matrix.amd64

.PHONY: all
all: native arm amd64

