# Prometheus Shelly Exporter and Grafana integration

This exporter exports some metrics from Shelly to Prometheus.

Tested with:
- Shelly H&T - only humidity and temperature
- WIP (1PM and 3EM)
 

## Build

    go get github.com/SchumacherFM/prometheus_shelly_exporter/
    cd $GOPATH/src/github.com/SchumacherFM/prometheus_shelly_exporter
    go install

## Docker

    $ docker pull ghcr.io/schumacherfm/shelly-exporter:latest

## Execution

Usage:

    $GOPATH/bin/prometheus_shelly_exporter -h

    $ go run main.go
    NAME:
    main - Converts received data from MQTT towards prometheus format
    
    USAGE:
    main [global options] command [command options] [arguments...]
    
    COMMANDS:
    debug    inspect topics and their data
    prom     forwards the mqtt data towards prometheus
    help, h  Shows a list of commands or help for one command
    
    GLOBAL OPTIONS:
    --mqtt-url value [ --mqtt-url value ]                mqtt://hostname:port [$MQTT_HOSTS]
    --mqtt-user value                                    mqtt username [$MQTT_USERNAME]
    --mqtt-pass value                                    mqtt password [$MQTT_PASSWORD]
    --topic value, -t value [ --topic value, -t value ]  MQTT Topics http://www.steves-internet-guide.com/understanding-mqtt-topics/ (default: "shellies/+/info")
    --verbose                                            (default: false)
    --help, -h                                           show help
 

## Grafana Dashboard
 
...
