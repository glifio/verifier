FROM rust:1.44-slim-buster AS builder
RUN apt update
RUN apt install -y make g++ git bash jq opencl-headers libclang-dev hwloc libhwloc-dev
WORKDIR /
ADD .gitmodules .gitmodules
ADD .git .git
ADD ./filecoin-ffi ./filecoin-ffi/
RUN git submodule update --init
RUN cd filecoin-ffi && make

FROM golang:1.16.0-buster AS builder-verifier
RUN apt update
RUN apt install -y pkg-config gcc mesa-opencl-icd ocl-icd-opencl-dev hwloc libhwloc-dev
WORKDIR /verifier
COPY --from=builder /filecoin-ffi ./filecoin-ffi/
ADD *.go ./
ADD go.mod go.sum ./
RUN go build -o /app .

FROM debian:buster-slim AS final
COPY --from=builder-verifier /etc/ssl/certs /etc/ssl/certs
COPY --from=builder-verifier /usr/lib/x86_64-linux-gnu/libOpenCL.so.1 /usr/lib/x86_64-linux-gnu/libOpenCL.so.1
COPY --from=builder-verifier /usr/lib/x86_64-linux-gnu/libhwloc.so.5 /usr/lib/x86_64-linux-gnu/libhwloc.so.5 
COPY --from=builder-verifier /usr/lib/x86_64-linux-gnu/libnuma.so.1 /usr/lib/x86_64-linux-gnu/libnuma.so.1
COPY --from=builder-verifier /usr/lib/x86_64-linux-gnu/libltdl.so.7 /usr/lib/x86_64-linux-gnu/libltdl.so.7
WORKDIR /verifier
COPY --from=builder-verifier /app .

ENTRYPOINT ./app
