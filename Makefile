all: dev


dev: build
	DEBUG=1 ./chillmailer

format:
	find . -iname '*.go' -exec go fmt {} \;

build:
	go build -o chillmailer .


