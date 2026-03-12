# use a minimal debian image
FROM debian:bookworm-slim

# define arguments and default values
ARG TARGETOS TARGETARCH
ARG USER_UID=10001

# create a non-root user for running the migrator
USER ${USER_UID}

# copy the binaries from the multi-stage build
COPY .build/${TARGETOS}-${TARGETARCH}/o11y-schema-migrator /o11y-schema-migrator

# run the binary as the entrypoint and pass the default dsn as a flag
ENTRYPOINT [ "/o11y-schema-migrator" ]
CMD ["--dsn", "tcp://localhost:9000"]
