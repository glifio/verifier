all:
	cd filecoin-ffi && make clean && cd ../
	make build VERSION=$(VERSION)
	make push

build:
	@echo building version: $(VERSION)
	docker build -f Dockerfile -t openworklabs/verifier:$(VERSION) .

push:
	docker push openworklabs/verifier

run:
	docker run --env-file env.list -d -p 8080:8080 --name verifier --restart always glif/verifier:0.0.3