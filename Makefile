.PHONY: native
native:
	go build -v -o matrix

.PHONY: arm
arm:
	GOOS=linux GOARCH=arm GOARM=5 go build -v -o matrix.arm

.PHONY: all
all: native arm

