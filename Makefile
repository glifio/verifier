all:
	make build VERSION=$(VERSION)
	make push

build:
	@echo building version: $(VERSION)
	docker build -f Dockerfile -t openworklabs/verifier$(VERSION) .

push:
	docker push openworklabs/verifier
