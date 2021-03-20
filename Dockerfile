from golang:1.16
run mkdir /slagios
copy . /slagios/
workdir /slagios
env GOFLAGS=-mod=vendor
run go build ./.../slagios

from debian:latest
run apt-get update \
  && apt-get install -y \
    nagios-plugins \
    nagios-plugins-contrib
copy --from=0 /slagios/slagios .
entrypoint [ "/slagios" ]
