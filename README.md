Very simple program that can run some nagios plugins on a schedule and
notify to slack when their state change.

## Running

The program is configured solely through environment variables:

To set up a check export:

    SLAGIOS_check_<check_name>=<check_comand>

where `<check_name>` could be something like "01" for the first check,
"02" for the second etc. But `<check_name>` can be any string.

Optionally set the interval for a check:

    SLAGIOS_interval_<check_name>=<check_interval>

Use the same `<check_name>` as for the check command you want to change.
`<check_interval>` is in nano seconds, but takes any string parseable by
go's `time.ParseDuration` function. Eg. `30s`, `1min` etc.

The default check interval is 60s, but this can be changed by setting:

    SLAGIOS_interval=<duration>

Valid values for `<duration>` are the same as above.

A Slack webhook URL is required:

    SLAGIOS_webhook=<url>

To run the docker image create a file containing environment variables as
described above, and run:

    docker run --name slagios --rm --env-file env hal9kdk/slagios

## Building

To build the docker image:

    docker build -t hal9kdk/slagios .
