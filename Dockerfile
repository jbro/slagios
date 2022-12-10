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
add https://raw.githubusercontent.com/thehunmonkgroup/nagios-plugin-newest-file-age/v1.1/check_newest_file_age /usr/lib/nagios/plugins/
add https://raw.githubusercontent.com/thehunmonkgroup/nagios-plugin-newest-file-age/v1.1/utils.sh /usr/lib/nagios/plugins/
run chmod +x /usr/lib/nagios/plugins/check_newest_file_age /usr/lib/nagios/plugins/utils.sh

copy --from=0 /slagios/slagios .
entrypoint [ "/slagios" ]
