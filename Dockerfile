FROM rust:1.44-buster AS builder-filecoin-ffi

RUN apt update
RUN apt install -y make git bash jq opencl-headers
ADD ./fil-blst ./fil-blst/
# need to run ./build.sh script here?
ADD ./filecoin-ffi ./filecoin-ffi/
WORKDIR /filecoin-ffi
RUN make



FROM golang:1.14.4-buster AS builder-verifier

RUN apt update
RUN apt install -y pkg-config gcc mesa-opencl-icd ocl-icd-opencl-dev
WORKDIR /verifier
COPY --from=builder-filecoin-ffi /filecoin-ffi ./filecoin-ffi/
COPY --from=builder-fil-blst /fil-blst ./fil-blst/
ADD *.go ./
ADD go.mod go.sum ./
RUN go build -o /app .

ENTRYPOINT /app
