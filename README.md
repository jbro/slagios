Very simple program that can run some nagios plugins on a schedule and
notify to slack when their state change.

## Running

The program is configured solely through environment variables:

To set up a check export:

    SLAGIOS_check_<check_name>=<check_comand>

where `<check_name>` could be something like "01" for the first check,
"02" for the second etc. But `<check_name>` can be any string as long as
it unique to a check.

Optionally set the check and recheck interval for a check:

    SLAGIOS_interval_<check_name>=<check_interval>
    SLAGIOS_rinterval_<check_name>=<check_interval>

Use the same `<check_name>` as for the check command you want to change.

The default check interval is 60s, but this can be changed by setting:

    SLAGIOS_interval=<check_interval>

The default recheck interval is 60s, but this can be changed by setting:

    SLAGIOS_rinterval=<check_interval>

`<check_interval>` is in nano seconds, but takes any string parseable by
go's `time.ParseDuration` function. Eg. `30s`, `1min` etc.

A Slack webhook URL is required if you want to send state changes to Slack:

    SLAGIOS_webhook=<url>

To enable a slash command create the slash command a point it at the
exposed port (port 80 internally), and set the following environment
variable with you Slack App secret key:

    SLAGIOS_signingkey=<signing_secret>


To run the docker image create a file containing environment variables as
described above, and run:

    docker run --name slagios --rm --env-file env -p 8000:80 jbros/slagios

## Building

To build the docker image:

    docker build -t jbros/slagios .
