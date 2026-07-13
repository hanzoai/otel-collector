# collect docker container logs and syslog using filelog reciever

Steps
 * Make sure you have clickhouse from o11y running on your system.
 * run `make build-o11y-collector` in the root directory of this project
 * run the `docker compose up -d`
 * generate logs `docker run --rm  mingrammer/flog:0.4.3 --format=json --sleep=0.5s --loop` 